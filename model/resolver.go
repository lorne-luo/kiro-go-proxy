// Package model provides model resolution and caching for Kiro Gateway.
// It implements a 4-layer resolution pipeline for model names.
package model

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"kiro-go-proxy/config"

	log "github.com/sirupsen/logrus"
)

// Resolution represents the result of model resolution
type Resolution struct {
	InternalID      string
	Source          string // "cache", "hidden", "alias", "passthrough"
	OriginalRequest string
	Normalized      string
	IsVerified      bool
}

// Info represents model information
type Info struct {
	ModelID string `json:"modelId"`
}

// Cache provides model information caching
type Cache struct {
	mu           sync.RWMutex
	models       map[string]Info
	maxInput     map[string]int
	lastUpdate   time.Time
	ttl          time.Duration
	hiddenModels map[string]string
}

// NewCache creates a new model cache
func NewCache(cfg *config.Config) *Cache {
	c := &Cache{
		models:       make(map[string]Info),
		maxInput:     make(map[string]int),
		ttl:          time.Duration(cfg.ModelCacheTTL) * time.Second,
		hiddenModels: cfg.HiddenModels,
	}

	// Initialize with fallback models
	for _, m := range cfg.FallbackModels {
		c.models[m.ModelID] = Info{ModelID: m.ModelID}
		c.maxInput[m.ModelID] = cfg.MaxInputTokens
	}

	return c
}

// Update updates the cache with model list from API
// Completely replaces existing cache contents with new data
func (c *Cache) Update(models []Info) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Replace entire cache (matches Python behavior)
	c.models = make(map[string]Info)
	for _, m := range models {
		c.models[m.ModelID] = m
	}
	c.lastUpdate = time.Now()

	log.Debugf("Model cache updated with %d models", len(models))
}

// IsValidModel checks if a model exists in cache
func (c *Cache) IsValidModel(modelID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.models[modelID]
	return ok
}

// GetAllModelIDs returns all model IDs
func (c *Cache) GetAllModelIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ids := make([]string, 0, len(c.models))
	for id := range c.models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// GetMaxInputTokens returns max input tokens for a model
func (c *Cache) GetMaxInputTokens(modelID string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if max, ok := c.maxInput[modelID]; ok {
		return max
	}
	return 200000 // default
}

// SetMaxInputTokens sets max input tokens for a model
func (c *Cache) SetMaxInputTokens(modelID string, maxTokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.maxInput[modelID] = maxTokens
}

// IsEmpty checks if the cache is empty
func (c *Cache) IsEmpty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.models) == 0
}

// Size returns the number of models in the cache
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.models)
}

// IsStale checks if the cache is stale (TTL exceeded or never updated)
func (c *Cache) IsStale() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.lastUpdate.IsZero() {
		return true
	}
	return time.Since(c.lastUpdate) > c.ttl
}

// LastUpdateTime returns the last update time
func (c *Cache) LastUpdateTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lastUpdate
}

// AddHiddenModel adds a hidden model to the cache
func (c *Cache) AddHiddenModel(displayName, internalID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.models[displayName] = Info{ModelID: displayName}
	c.hiddenModels[displayName] = internalID
}

// Resolver resolves model names to internal Kiro IDs
type Resolver struct {
	cache          *Cache
	hiddenModels   map[string]string
	aliases        map[string]string
	hiddenFromList map[string]bool
}

// NewResolver creates a new model resolver
func NewResolver(cache *Cache, cfg *config.Config) *Resolver {
	hiddenFromList := make(map[string]bool)
	for _, id := range cfg.HiddenFromList {
		hiddenFromList[id] = true
	}

	return &Resolver{
		cache:          cache,
		hiddenModels:   cfg.HiddenModels,
		aliases:        cfg.ModelAliases,
		hiddenFromList: hiddenFromList,
	}
}

// Resolve resolves external model name to internal Kiro ID
func (r *Resolver) Resolve(externalModel string) *Resolution {
	// Layer 0: Resolve alias
	resolvedModel := externalModel
	if alias, ok := r.aliases[externalModel]; ok {
		resolvedModel = alias
		log.Debugf("Alias resolved: '%s' → '%s'", externalModel, resolvedModel)
	}

	// Layer 1: Normalize name
	normalized := NormalizeModelName(resolvedModel)
	log.Debugf("Model resolution: '%s' → normalized: '%s'", externalModel, normalized)

	// Layer 2: Check dynamic cache
	if r.cache.IsValidModel(normalized) {
		log.Debugf("Model '%s' found in dynamic cache", normalized)
		return &Resolution{
			InternalID:      normalized,
			Source:          "cache",
			OriginalRequest: externalModel,
			Normalized:      normalized,
			IsVerified:      true,
		}
	}

	// Layer 3: Check hidden models
	if internalID, ok := r.hiddenModels[normalized]; ok {
		log.Debugf("Model '%s' found in hidden models → '%s'", normalized, internalID)
		return &Resolution{
			InternalID:      internalID,
			Source:          "hidden",
			OriginalRequest: externalModel,
			Normalized:      normalized,
			IsVerified:      true,
		}
	}

	// Layer 4: Pass-through - let Kiro decide
	log.Infof("Model '%s' (normalized: '%s') not in cache, passing through to Kiro API",
		externalModel, normalized)
	return &Resolution{
		InternalID:      normalized,
		Source:          "passthrough",
		OriginalRequest: externalModel,
		Normalized:      normalized,
		IsVerified:      false,
	}
}

// GetAvailableModels returns all available model IDs for /v1/models endpoint
func (r *Resolver) GetAvailableModels() []string {
	models := make(map[string]bool)

	// Add cache models
	for _, id := range r.cache.GetAllModelIDs() {
		if !r.hiddenFromList[id] {
			models[id] = true
		}
	}

	// Add hidden model display names
	for displayName := range r.hiddenModels {
		if !r.hiddenFromList[displayName] {
			models[displayName] = true
		}
	}

	// Add aliases
	for alias := range r.aliases {
		models[alias] = true
	}

	// Convert to sorted slice
	result := make([]string, 0, len(models))
	for id := range models {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

// GetModelsByFamily returns models filtered by family
func (r *Resolver) GetModelsByFamily(family string) []string {
	allModels := r.GetAvailableModels()
	var result []string
	for _, m := range allModels {
		if strings.Contains(strings.ToLower(m), strings.ToLower(family)) {
			result = append(result, m)
		}
	}
	return result
}

// GetSuggestionsForModel returns models from the same family for error messages
func (r *Resolver) GetSuggestionsForModel(modelName string) []string {
	family := ExtractModelFamily(modelName)
	if family != "" {
		return r.GetModelsByFamily(family)
	}
	return r.GetAvailableModels()
}

// NormalizeModelName normalizes client model name to Kiro format
func NormalizeModelName(name string) string {
	if name == "" {
		return name
	}

	nameLower := strings.ToLower(name)

	// Pattern 1: Standard format - claude-{family}-{major}-{minor}(-{suffix})?
	// e.g., claude-haiku-4-5, claude-haiku-4-5-20251001
	standardPattern := regexp.MustCompile(`^(claude-(?:haiku|sonnet|opus)-\d+)-(\d{1,2})(?:-(?:\d{8}|latest|\d+))?$`)
	if match := standardPattern.FindStringSubmatch(nameLower); match != nil {
		return match[1] + "." + match[2] // claude-haiku-4.5
	}

	// Pattern 2: Standard format without minor - claude-{family}-{major}(-{date})?
	// e.g., claude-sonnet-4, claude-sonnet-4-20250514
	noMinorPattern := regexp.MustCompile(`^(claude-(?:haiku|sonnet|opus)-\d+)(?:-\d{8})?$`)
	if match := noMinorPattern.FindStringSubmatch(nameLower); match != nil {
		return match[1]
	}

	// Pattern 3: Legacy format - claude-{major}-{minor}-{family}(-{suffix})?
	// e.g., claude-3-7-sonnet, claude-3-7-sonnet-20250219
	legacyPattern := regexp.MustCompile(`^(claude)-(\d+)-(\d+)-(haiku|sonnet|opus)(?:-(?:\d{8}|latest|\d+))?$`)
	if match := legacyPattern.FindStringSubmatch(nameLower); match != nil {
		return match[1] + "-" + match[2] + "." + match[3] + "-" + match[4] // claude-3.7-sonnet
	}

	// Pattern 4: Already normalized with dot but has date suffix
	// e.g., claude-haiku-4.5-20251001
	dotWithDatePattern := regexp.MustCompile(`^(claude-(?:\d+\.\d+-)?(?:haiku|sonnet|opus)(?:-\d+\.\d+)?)-\d{8}$`)
	if match := dotWithDatePattern.FindStringSubmatch(nameLower); match != nil {
		return match[1]
	}

	// Pattern 5: Inverted format with suffix - claude-{major}.{minor}-{family}-{suffix}
	// e.g., claude-4.5-opus-high
	invertedPattern := regexp.MustCompile(`^claude-(\d+)\.(\d+)-(haiku|sonnet|opus)-(.+)$`)
	if match := invertedPattern.FindStringSubmatch(nameLower); match != nil {
		return "claude-" + match[3] + "-" + match[1] + "." + match[2] // claude-opus-4.5
	}

	return name
}

// ExtractModelFamily extracts model family from model name
func ExtractModelFamily(modelName string) string {
	familyMatch := regexp.MustCompile(`(?i)(haiku|sonnet|opus)`).FindStringSubmatch(modelName)
	if len(familyMatch) > 1 {
		return strings.ToLower(familyMatch[1])
	}
	return ""
}

// GetModelIDForKiro gets the model ID to send to Kiro API
func GetModelIDForKiro(modelName string, hiddenModels map[string]string) string {
	normalized := NormalizeModelName(modelName)
	if internalID, ok := hiddenModels[normalized]; ok {
		return internalID
	}
	return normalized
}
