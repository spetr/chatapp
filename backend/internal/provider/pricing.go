package provider

import (
	"github.com/spetr/chatapp/internal/models"
	"strings"
)

/*
================================================================================
                    OLLAMA LOCAL INFERENCE COST CALCULATION
================================================================================

This module calculates the operational cost of running LLM inference locally
using Ollama, based on electricity consumption.

CALCULATION METHOD:
─────────────────────────────────────────────────────────────────────────────────

1. POWER CONSUMPTION
   Total Power = GPU TDP × PUE (Power Usage Effectiveness)

   - TDP: Thermal Design Power of GPU (manufacturer spec)
   - PUE: Overhead for cooling, PSU efficiency, etc.
     • 1.0 = no overhead (theoretical)
     • 1.2 = typical home/office setup
     • 1.3-1.5 = datacenter with cooling

2. HOURLY COST
   Cost/Hour = (Total Power in kW) × Electricity Rate ($/kWh)

   Example for RTX 4090:
   • TDP: 450W, PUE: 1.2 → Total: 540W = 0.54 kW
   • At $0.12/kWh → $0.065/hour

3. TOKEN THROUGHPUT
   Two different speeds apply:
   • Prompt Processing (Input): Fast parallel processing (~2000 tok/s on 4090)
   • Generation (Output): Sequential, slower (~100 tok/s on 4090)

4. COST PER MILLION TOKENS
   Cost/1M = (Cost/Hour ÷ Tokens/Hour) × 1,000,000

   Example for RTX 4090 at $0.12/kWh:
   • Input:  $0.065/hr ÷ 7.2M tok/hr = $0.009/1M tokens
   • Output: $0.065/hr ÷ 360K tok/hr = $0.18/1M tokens

COMPARISON WITH CLOUD APIs:
─────────────────────────────────────────────────────────────────────────────────

Provider          │ Model              │ Input/1M  │ Output/1M
──────────────────┼────────────────────┼───────────┼───────────
Ollama (RTX 4090) │ Any local model    │ $0.009    │ $0.18
Ollama (H100)     │ Any local model    │ $0.003    │ $0.08
Anthropic         │ Claude 3.5 Sonnet  │ $3.00     │ $15.00
OpenAI            │ GPT-4o             │ $2.50     │ $10.00
OpenAI            │ GPT-4o-mini        │ $0.15     │ $0.60

NOTE: Token speeds are approximations for ~70B parameter models.
      Smaller models (7B, 13B) will be significantly faster.
      Larger models (405B) require multi-GPU and are slower.

================================================================================
*/

// ModelPricing contains per-million token costs in USD
type ModelPricing struct {
	InputPer1M  float64 // Cost per 1M input tokens
	OutputPer1M float64 // Cost per 1M output tokens
}

// GPUSpec contains GPU specifications for electricity cost calculation
type GPUSpec struct {
	Name            string // Display name
	TDP             int    // Thermal Design Power in watts
	PromptTokPerSec int    // Approximate prompt processing speed (tokens/sec)
	GenTokPerSec    int    // Approximate generation speed (tokens/sec)
	VRAM            int    // VRAM in GB
}

// Available GPU options for Ollama local inference
// Token speeds are approximations for ~70B parameter models (e.g., Llama 3 70B)
// Smaller models will be faster, larger models slower
var GPUOptions = map[string]GPUSpec{
	// NVIDIA Consumer
	"rtx-5090": {
		Name:            "NVIDIA RTX 5090",
		TDP:             575,
		PromptTokPerSec: 2500,
		GenTokPerSec:    120,
		VRAM:            32,
	},
	"rtx-4090": {
		Name:            "NVIDIA RTX 4090",
		TDP:             450,
		PromptTokPerSec: 2000,
		GenTokPerSec:    100,
		VRAM:            24,
	},
	"rtx-4080": {
		Name:            "NVIDIA RTX 4080",
		TDP:             320,
		PromptTokPerSec: 1500,
		GenTokPerSec:    70,
		VRAM:            16,
	},
	"rtx-4070-ti": {
		Name:            "NVIDIA RTX 4070 Ti",
		TDP:             285,
		PromptTokPerSec: 1200,
		GenTokPerSec:    55,
		VRAM:            12,
	},
	"rtx-3090": {
		Name:            "NVIDIA RTX 3090",
		TDP:             350,
		PromptTokPerSec: 1400,
		GenTokPerSec:    65,
		VRAM:            24,
	},
	"rtx-3080": {
		Name:            "NVIDIA RTX 3080",
		TDP:             320,
		PromptTokPerSec: 1100,
		GenTokPerSec:    50,
		VRAM:            10,
	},

	// NVIDIA Datacenter
	"a100-80gb": {
		Name:            "NVIDIA A100 80GB",
		TDP:             400,
		PromptTokPerSec: 4000,
		GenTokPerSec:    150,
		VRAM:            80,
	},
	"a100-40gb": {
		Name:            "NVIDIA A100 40GB",
		TDP:             400,
		PromptTokPerSec: 3500,
		GenTokPerSec:    130,
		VRAM:            40,
	},
	"h100": {
		Name:            "NVIDIA H100",
		TDP:             700,
		PromptTokPerSec: 8000,
		GenTokPerSec:    300,
		VRAM:            80,
	},
	"l40s": {
		Name:            "NVIDIA L40S",
		TDP:             350,
		PromptTokPerSec: 3000,
		GenTokPerSec:    120,
		VRAM:            48,
	},

	// AMD
	"rx-7900-xtx": {
		Name:            "AMD RX 7900 XTX",
		TDP:             355,
		PromptTokPerSec: 1000,
		GenTokPerSec:    45,
		VRAM:            24,
	},
	"mi300x": {
		Name:            "AMD MI300X",
		TDP:             750,
		PromptTokPerSec: 7000,
		GenTokPerSec:    280,
		VRAM:            192,
	},

	// Apple Silicon
	"m2-ultra": {
		Name:            "Apple M2 Ultra",
		TDP:             100, // Estimated, Apple doesn't disclose
		PromptTokPerSec: 800,
		GenTokPerSec:    35,
		VRAM:            192, // Unified memory
	},
	"m3-max": {
		Name:            "Apple M3 Max",
		TDP:             60,
		PromptTokPerSec: 600,
		GenTokPerSec:    30,
		VRAM:            128,
	},
	"m4-max": {
		Name:            "Apple M4 Max",
		TDP:             70,
		PromptTokPerSec: 900,
		GenTokPerSec:    45,
		VRAM:            128,
	},
}

// OllamaConfig holds configuration for Ollama pricing calculation
type OllamaConfig struct {
	GPU             string  // GPU model key from GPUOptions
	ElectricityRate float64 // $/kWh
	PUE             float64 // Power Usage Effectiveness (1.0 = no overhead, 1.3 = typical datacenter)
}

// DefaultOllamaConfig provides reasonable defaults
var DefaultOllamaConfig = OllamaConfig{
	GPU:             "rtx-4090",
	ElectricityRate: 0.12, // $0.12/kWh average
	PUE:             1.2,  // Home setup with some cooling
}

// CalculateOllamaPricing calculates pricing based on GPU and electricity costs
func CalculateOllamaPricing(config OllamaConfig) ModelPricing {
	gpu, ok := GPUOptions[config.GPU]
	if !ok {
		gpu = GPUOptions["rtx-4090"] // Fallback
	}

	// Total power with PUE overhead
	totalWatts := float64(gpu.TDP) * config.PUE

	// Cost per hour
	costPerHour := (totalWatts / 1000) * config.ElectricityRate

	// Tokens per hour
	promptTokPerHour := float64(gpu.PromptTokPerSec) * 3600
	genTokPerHour := float64(gpu.GenTokPerSec) * 3600

	// Cost per 1M tokens
	inputPer1M := (costPerHour / promptTokPerHour) * 1_000_000
	outputPer1M := (costPerHour / genTokPerHour) * 1_000_000

	return ModelPricing{
		InputPer1M:  inputPer1M,
		OutputPer1M: outputPer1M,
	}
}

// pricingTable maps model prefixes to their pricing
// Prices as of January 2025
var pricingTable = map[string]map[string]ModelPricing{
	// Anthropic Claude models
	"claude": {
		// Claude 4.5 (latest)
		"claude-sonnet-4-5": {InputPer1M: 3.00, OutputPer1M: 15.00},
		"claude-haiku-4-5":  {InputPer1M: 1.00, OutputPer1M: 5.00},
		"claude-opus-4-5":   {InputPer1M: 5.00, OutputPer1M: 25.00},
		// Claude 4 (legacy)
		"claude-sonnet-4":   {InputPer1M: 3.00, OutputPer1M: 15.00},
		"claude-opus-4":     {InputPer1M: 15.00, OutputPer1M: 75.00},
		"claude-opus-4-1":   {InputPer1M: 15.00, OutputPer1M: 75.00},
		// Claude 3.x (legacy)
		"claude-3-7-sonnet": {InputPer1M: 3.00, OutputPer1M: 15.00},
		"claude-3-5-sonnet": {InputPer1M: 3.00, OutputPer1M: 15.00},
		"claude-3-5-haiku":  {InputPer1M: 0.80, OutputPer1M: 4.00},
		"claude-3-opus":     {InputPer1M: 15.00, OutputPer1M: 75.00},
		"claude-3-sonnet":   {InputPer1M: 3.00, OutputPer1M: 15.00},
		"claude-3-haiku":    {InputPer1M: 0.25, OutputPer1M: 1.25},
		"_default":          {InputPer1M: 3.00, OutputPer1M: 15.00}, // Sonnet as default
	},

	// OpenAI models
	"openai": {
		"gpt-4o":        {InputPer1M: 2.50, OutputPer1M: 10.00},
		"gpt-4o-mini":   {InputPer1M: 0.15, OutputPer1M: 0.60},
		"gpt-4-turbo":   {InputPer1M: 10.00, OutputPer1M: 30.00},
		"gpt-4":         {InputPer1M: 30.00, OutputPer1M: 60.00},
		"gpt-3.5-turbo": {InputPer1M: 0.50, OutputPer1M: 1.50},
		"o1":            {InputPer1M: 15.00, OutputPer1M: 60.00},
		"o1-mini":       {InputPer1M: 3.00, OutputPer1M: 12.00},
		"o1-preview":    {InputPer1M: 15.00, OutputPer1M: 60.00},
		"o3":            {InputPer1M: 15.00, OutputPer1M: 60.00},
		"o3-mini":       {InputPer1M: 1.10, OutputPer1M: 4.40},
		"o4-mini":       {InputPer1M: 1.10, OutputPer1M: 4.40},
		"_default":      {InputPer1M: 2.50, OutputPer1M: 10.00}, // GPT-4o as default
	},
}

// Current Ollama config - can be updated at runtime
var currentOllamaConfig = DefaultOllamaConfig

// SetOllamaConfig updates the Ollama pricing configuration
func SetOllamaConfig(config OllamaConfig) {
	currentOllamaConfig = config
}

// GetOllamaConfig returns the current Ollama configuration
func GetOllamaConfig() OllamaConfig {
	return currentOllamaConfig
}

// GetModelPricing returns pricing for a specific provider and model
func GetModelPricing(providerName, modelName string) ModelPricing {
	providerName = strings.ToLower(providerName)
	modelName = strings.ToLower(modelName)

	// Special handling for local providers - calculate from GPU specs
	// Both Ollama and llama.cpp use the same local inference engine
	if providerName == "ollama" || providerName == "llamacpp" {
		return CalculateOllamaPricing(currentOllamaConfig)
	}

	// Use the model registry as primary source
	registry := models.GetRegistry()
	registryPricing := registry.GetPricing(providerName, modelName)

	// If registry has valid pricing, use it
	if registryPricing.InputPer1M > 0 || registryPricing.OutputPer1M > 0 {
		return ModelPricing{
			InputPer1M:  registryPricing.InputPer1M,
			OutputPer1M: registryPricing.OutputPer1M,
		}
	}

	// Fallback to legacy pricingTable for backwards compatibility
	providerPricing, ok := pricingTable[providerName]
	if !ok {
		// Unknown provider - return zero (free/unknown)
		return ModelPricing{InputPer1M: 0, OutputPer1M: 0}
	}

	// Try exact match first
	if pricing, ok := providerPricing[modelName]; ok {
		return pricing
	}

	// Try prefix matching for versioned models (e.g., "gpt-4o-2024-08-06" -> "gpt-4o")
	for prefix, pricing := range providerPricing {
		if prefix != "_default" && strings.HasPrefix(modelName, prefix) {
			return pricing
		}
	}

	// Return provider default
	if defaultPricing, ok := providerPricing["_default"]; ok {
		return defaultPricing
	}

	return ModelPricing{InputPer1M: 0, OutputPer1M: 0}
}

// CalculateCost calculates the total cost for a request given token counts
func CalculateCost(providerName, modelName string, inputTokens, outputTokens int) float64 {
	pricing := GetModelPricing(providerName, modelName)

	inputCost := float64(inputTokens) / 1_000_000 * pricing.InputPer1M
	outputCost := float64(outputTokens) / 1_000_000 * pricing.OutputPer1M

	return inputCost + outputCost
}

// CalculateInputCost calculates cost for input tokens only
func CalculateInputCost(providerName, modelName string, inputTokens int) float64 {
	pricing := GetModelPricing(providerName, modelName)
	return float64(inputTokens) / 1_000_000 * pricing.InputPer1M
}

// IsLocalProvider returns true if the provider runs locally (no cloud costs)
func IsLocalProvider(providerName string) bool {
	name := strings.ToLower(providerName)
	return name == "ollama" || name == "llamacpp"
}

// GetGPUList returns list of available GPU options
func GetGPUList() map[string]GPUSpec {
	return GPUOptions
}
