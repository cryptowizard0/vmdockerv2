package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/utils"
	serverSchema "github.com/hymatrix/hymx/server/schema"
	vmmSchema "github.com/hymatrix/hymx/vmm/schema"
	"github.com/permadao/goar/schema"
	goarSchema "github.com/permadao/goar/schema"
)

var (
	OpenclawModuleID = os.Getenv("OPENCLAW_MODULE_ID")
)

func buildOpenclawSpawnTags(model, provider, apiKey, gatewayToken, runtimeBackend string) []goarSchema.Tag {
	defaultModel := GetEnvWith("OPENCLAW_DEFAULT_MODEL", model)
	defaultProvider := GetEnvWith("OPENCLAW_DEFAULT_PROVIDER", provider)

	tags := make([]goarSchema.Tag, 0, 8)
	if provider != "" {
		tags = append(tags, goarSchema.Tag{Name: "provider", Value: provider})
	}
	tags = append(tags,
		goarSchema.Tag{Name: "model", Value: model},
		goarSchema.Tag{Name: "apiKey", Value: apiKey},
		goarSchema.Tag{Name: utils.ContainerEnvTagPrefix + "OPENCLAW_GATEWAY_TOKEN", Value: gatewayToken},
	)
	tags = append(tags, runtimeBackendTags(runtimeBackend)...)
	// RUNTIME_TYPE is no longer baked into the image; openclaw must declare it.
	tags = append(tags, runtimeTypeTags("openclaw")...)
	if defaultModel != "" {
		tags = append(tags, goarSchema.Tag{Name: utils.ContainerEnvTagPrefix + "OPENCLAW_DEFAULT_MODEL", Value: defaultModel})
	}
	if defaultProvider != "" {
		tags = append(tags, goarSchema.Tag{Name: utils.ContainerEnvTagPrefix + "OPENCLAW_DEFAULT_PROVIDER", Value: defaultProvider})
	}
	return tags
}

func spawnOpenclaw() string {
	if OpenclawModuleID == "" {
		OpenclawModuleID = GetEnvWith("OPENCLAW_MODULE_ID", "AzbJ2MZ7hz5gJnbhWFbQ9JJRRJe4VdVtDAePKUjC6zM")
	}

	openclawModel := GetEnvWith("OPENCLAW_MODEL", "")
	openclawProvider := GetEnvWith("OPENCLAW_PROVIDER", "")
	openclawAPIKey := os.Getenv("OPENCLAW_API_KEY")
	openclawGatewayToken := GetEnvWith("OPENCLAW_GATEWAY_TOKEN", "openclaw-test-token")
	runtimeBackend := GetEnvWith("RUNTIME_BACKEND", "")

	start := time.Now()
	fmt.Printf("[openclaw_spawn] start=%s module=%s\n", start.Format(time.RFC3339), OpenclawModuleID)
	resp, err := s.Spawn(
		OpenclawModuleID,
		scheduler,
		buildOpenclawSpawnTags(openclawModel, openclawProvider, openclawAPIKey, openclawGatewayToken, runtimeBackend),
	)
	if err != nil {
		fmt.Printf("[openclaw_spawn] failed after=%s err=%v\n", time.Since(start), err)
		return ""
	}
	res, err := waitForResponse(resp.Id, resp.Id, openclawWaitTimeout("OPENCLAW_SPAWN_WAIT_TIMEOUT", 10*time.Minute))
	if err != nil {
		fmt.Printf("[openclaw_spawn] failed after=%s err=%v\n", time.Since(start), err)
		return ""
	}

	target := res.Id
	fmt.Printf("[openclaw_spawn] done=%s elapsed=%s pid=%s\n", time.Now().Format(time.RFC3339), time.Since(start), target)

	return target
}

func chatOpenclaw(target string) {
	start := time.Now()
	fmt.Printf("[openclaw_chat] start=%s target=%s\n", start.Format(time.RFC3339), target)
	resp, err := s.SendMessage(target, "",
		[]schema.Tag{
			{Name: "Action", Value: "Chat"},
			{Name: "Command", Value: "你好"},
		})
	if err != nil {
		fmt.Printf("[openclaw_chat] failed after=%s target=%s err=%v\n", time.Since(start), target, err)
		return
	}
	res, err := waitForResponse(target, resp.Id, openclawWaitTimeout("OPENCLAW_MESSAGE_WAIT_TIMEOUT", 5*time.Minute))
	if err != nil {
		fmt.Printf("[openclaw_chat] failed after=%s target=%s err=%v\n", time.Since(start), target, err)
		return
	}
	fmt.Printf("[openclaw_chat] done=%s elapsed=%s target=%s\n", time.Now().Format(time.RFC3339), time.Since(start), target)
	fmt.Println("res, ", res)

}

func telegramOpenclaw(target string) {
	botToken := GetEnv("OPENCLAW_TELEGRAM_BOT_TOKEN")
	defaultAccount := GetEnvWith("OPENCLAW_TELEGRAM_DEFAULT_ACCOUNT", "main")
	dmPolicy := GetEnvWith("OPENCLAW_TELEGRAM_DM_POLICY", "pairing")
	allowFrom := GetEnvWith("OPENCLAW_TELEGRAM_ALLOW_FROM", "")

	start := time.Now()
	fmt.Printf("[openclaw_tg] start=%s target=%s\n", start.Format(time.RFC3339), target)
	tags := []schema.Tag{
		{Name: "Action", Value: "ConfigureTelegram"},
		{Name: "botToken", Value: botToken},
		{Name: "defaultAccount", Value: defaultAccount},
		{Name: "dmPolicy", Value: dmPolicy},
	}
	if allowFrom != "" {
		tags = append(tags, schema.Tag{Name: "allowFrom", Value: allowFrom})
	}
	resp, err := s.SendMessage(target, "", tags)
	if err != nil {
		fmt.Printf("[openclaw_tg] failed after=%s target=%s err=%v\n", time.Since(start), target, err)
		return
	}
	res, err := waitForResponse(target, resp.Id, openclawWaitTimeout("OPENCLAW_MESSAGE_WAIT_TIMEOUT", 5*time.Minute))
	if err != nil {
		fmt.Printf("[openclaw_tg] failed after=%s target=%s err=%v\n", time.Since(start), target, err)
		return
	}
	fmt.Printf("[openclaw_tg] done=%s elapsed=%s target=%s\n", time.Now().Format(time.RFC3339), time.Since(start), target)
	fmt.Println("res, ", res)

}

func pairTgOpenclaw(target, pairCode string) {
	start := time.Now()
	fmt.Printf("[openclaw_pair] start=%s target=%s\n", start.Format(time.RFC3339), target)
	resp, err := s.SendMessage(target, "",
		[]schema.Tag{
			{Name: "Action", Value: "ApproveTelegramPairing"},
			{Name: "code", Value: pairCode},
			{Name: "channel", Value: "telegram"},
			{Name: "dmPolicy", Value: "pairing"},
		})
	if err != nil {
		fmt.Printf("[openclaw_pair] failed after=%s target=%s err=%v\n", time.Since(start), target, err)
		return
	}
	res, err := waitForResponse(target, resp.Id, openclawWaitTimeout("OPENCLAW_MESSAGE_WAIT_TIMEOUT", 5*time.Minute))
	if err != nil {
		fmt.Printf("[openclaw_pair] failed after=%s target=%s err=%v\n", time.Since(start), target, err)
		return
	}
	fmt.Printf("[openclaw_pair] done=%s elapsed=%s target=%s\n", time.Now().Format(time.RFC3339), time.Since(start), target)
	fmt.Println("res, ", res)

}

func waitForResponse(pid, msgid string, timeout time.Duration) (*serverSchema.Response, error) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for result after %s", timeout)
		case <-ticker.C:
			result, err := s.Client.GetResult(pid, msgid)
			if err != nil {
				return nil, err
			}
			if result.ItemId == "" {
				continue
			}
			payload, err := json.Marshal(result)
			if err != nil {
				return nil, err
			}
			return &serverSchema.Response{
				Id:      responseID(result, msgid),
				Message: string(payload),
			}, nil
		}
	}
}

func responseID(result vmmSchema.VmmResult, fallback string) string {
	if result.ItemId != "" {
		return result.ItemId
	}
	return fallback
}

func openclawWaitTimeout(envKey string, fallback time.Duration) time.Duration {
	raw := GetEnvWith(envKey, "")
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		fmt.Printf("[%s] invalid duration %q, fallback=%s\n", envKey, raw, fallback)
		return fallback
	}
	return parsed
}
