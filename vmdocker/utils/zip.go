package utils

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CompressDirectory compresses the specified directory into a tar.gz format string.
// It preserves the directory structure and file permissions.
// srcPath: the absolute path of the directory to be compressed
// Returns the compressed data as a string and any error encountered
func CompressDirectory(srcPath string) (string, error) {
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	// Walk through the directory and add files to tar
	err := filepath.Walk(srcPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(srcPath, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// If it's a regular file, write its contents
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(tw, file)
			if err != nil {
				return err
			}
		}

		return nil
	})

	// Close writers
	if err = tw.Close(); err != nil {
		return "", err
	}
	if err = gw.Close(); err != nil {
		return "", err
	}

	// return base64 encoded string
	return base64.StdEncoding.EncodeToString(buf.Bytes()), err
}

// DecompressToDirectory extracts a tar.gz format string to the specified directory.
// It recreates the original directory structure and preserves file permissions.
// data: the compressed data string
// destPath: the absolute path where the data should be extracted to
// Returns any error encountered during the extraction process
func DecompressToDirectory(data string, destPath string) error {
	// decode from base64
	decodedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return fmt.Errorf("failed to decode base64 data: %v", err)
	}

	// Create destination directory
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return err
	}
	destPath = filepath.Clean(destPath)

	// Create gzip reader
	gr, err := gzip.NewReader(bytes.NewReader(decodedData))
	if err != nil {
		return err
	}
	defer gr.Close()

	// Create tar reader
	tr := tar.NewReader(gr)

	// Iterate through the tar file
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		headerName := filepath.Clean(header.Name)
		if filepath.IsAbs(header.Name) || headerName == ".." || strings.HasPrefix(headerName, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("invalid archive path: %q", header.Name)
		}

		target := filepath.Join(destPath, headerName)
		rel, err := filepath.Rel(destPath, target)
		if err != nil {
			return fmt.Errorf("invalid archive path %q: %v", header.Name, err)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("archive path escapes destination: %q", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			// Create file
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// Write file contents
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		}
	}

	return nil
}
