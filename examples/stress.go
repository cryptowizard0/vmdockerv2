package main

import (
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"
)

func doTansfer() {
	defer s.Close()

	start := time.Now()
	var wg sync.WaitGroup

	concurrency := 5000
	sendCount := 5
	var successCount int32

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < sendCount; j++ {
				err := transfer(s, "UB0yJx53xBo_rFA4CvKP-WKO25M7kIGrqm2caarghkc", big.NewInt(1))
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				} else {
					fmt.Println("transfer failed:", err)
				}
			}

		}()
	}

	wg.Wait()
	elapsed := time.Since(start).Seconds()

	tps := float64(concurrency*sendCount) / elapsed
	fmt.Println("Concurrency: ", concurrency, ", Loops per goroutine: ", sendCount, ", Total transactions: ", concurrency*sendCount, ", Success: ", successCount)
	fmt.Printf("Total time: %.2fs, TPS: %.2f\n", elapsed, tps)
}
