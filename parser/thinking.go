// Package parser provides parsers for AWS Event Stream format and thinking blocks.
package parser

import (
	"strings"

	log "github.com/sirupsen/logrus"
)

// ThinkingHandlingMode defines how to handle thinking blocks
type ThinkingHandlingMode string

const (
	ThinkingHandlingAsReasoningContent ThinkingHandlingMode = "as_reasoning_content"
	ThinkingHandlingRemove             ThinkingHandlingMode = "remove"
	ThinkingHandlingPass               ThinkingHandlingMode = "pass"
	ThinkingHandlingStripTags          ThinkingHandlingMode = "strip_tags"
)

// ThinkingParseResult represents the result of parsing thinking content
type ThinkingParseResult struct {
	ThinkingContent        string
	RegularContent         string
	IsFirstThinkingChunk   bool
	IsLastThinkingChunk    bool
}

// ThinkingParser parses thinking/reasoning blocks from model output
type ThinkingParser struct {
	handlingMode      ThinkingHandlingMode
	openTags          []string
	initialBufferSize int
	foundThinking     bool

	// State
	buffer            string
	inThinking        bool
	thinkingContent   string
	regularContent    string
	thinkingTagOpen   string
	thinkingTagClose  string
	thinkingStarted   bool
	thinkingEnded     bool
	firstThinkingSent bool
}

// NewThinkingParser creates a new thinking parser
func NewThinkingParser(handlingMode ThinkingHandlingMode, openTags []string, initialBufferSize int) *ThinkingParser {
	if len(openTags) == 0 {
		openTags = []string{"<thinking>", "alettek", "<reasoning>", "<thought>"}
	}

	return &ThinkingParser{
		handlingMode:      handlingMode,
		openTags:          openTags,
		initialBufferSize: initialBufferSize,
	}
}

// Feed processes content and returns parsed result
func (p *ThinkingParser) Feed(content string) *ThinkingParseResult {
	result := &ThinkingParseResult{}

	// If we already found and finished thinking, pass through
	if p.thinkingEnded {
		result.RegularContent = content
		return result
	}

	// If we're inside thinking block
	if p.inThinking {
		p.processThinkingContent(content, result)
		return result
	}

	// If we haven't found thinking yet, check for tag
	if !p.foundThinking {
		p.buffer += content

		// Check if we have enough content to detect tag
		if len(p.buffer) >= p.initialBufferSize {
			p.checkForThinkingTag(result)
		} else {
			// Wait for more content
			return result
		}
	} else {
		// Already in thinking mode
		p.processThinkingContent(content, result)
	}

	return result
}

func (p *ThinkingParser) checkForThinkingTag(result *ThinkingParseResult) {
	for _, tag := range p.openTags {
		if strings.Contains(p.buffer, tag) {
			p.foundThinking = true
			p.inThinking = true
			p.thinkingTagOpen = tag
			p.thinkingTagClose = p.getCloseTag(tag)

			log.Debugf("Found thinking tag: %s", tag)

			// Split buffer at tag
			idx := strings.Index(p.buffer, tag)
			beforeTag := p.buffer[:idx]
			afterTag := p.buffer[idx+len(tag):]

			// Content before tag is regular
			if beforeTag != "" {
				result.RegularContent = beforeTag
			}

			// Content after tag is thinking
			if afterTag != "" {
				p.processThinkingContent(afterTag, result)
			}

			p.thinkingStarted = true
			result.IsFirstThinkingChunk = true
			return
		}
	}

	// No tag found, pass buffer through
	result.RegularContent = p.buffer
	p.buffer = ""
}

func (p *ThinkingParser) processThinkingContent(content string, result *ThinkingParseResult) {
	if p.thinkingTagClose == "" {
		return
	}

	// Check for closing tag
	if strings.Contains(content, p.thinkingTagClose) {
		idx := strings.Index(content, p.thinkingTagClose)
		thinkingPart := content[:idx]
		regularPart := content[idx+len(p.thinkingTagClose):]

		p.thinkingContent += thinkingPart
		p.inThinking = false
		p.thinkingEnded = true

		result.ThinkingContent = p.processForOutput(thinkingPart, !p.firstThinkingSent, true)
		result.IsLastThinkingChunk = true

		if regularPart != "" {
			result.RegularContent = regularPart
		}

		log.Debug("Thinking block processing completed")
	} else {
		p.thinkingContent += content
		result.ThinkingContent = p.processForOutput(content, !p.firstThinkingSent, false)
		if !p.firstThinkingSent {
			result.IsFirstThinkingChunk = true
			p.firstThinkingSent = true
		}
	}
}

func (p *ThinkingParser) getCloseTag(openTag string) string {
	switch openTag {
	case "alettek":
		return "alettek"
	case "<thinking>":
		return "</thinking>"
	case "<reasoning>":
		return "</reasoning>"
	case "<thought>":
		return "</thought>"
	default:
		return "</" + openTag[1:]
	}
}

// processForOutput processes content for output based on handling mode
func (p *ThinkingParser) processForOutput(content string, isFirst, isLast bool) string {
	switch p.handlingMode {
	case ThinkingHandlingRemove:
		return "" // Remove thinking content
	case ThinkingHandlingPass:
		// Include tags in content
		if isFirst {
			content = p.thinkingTagOpen + content
		}
		if isLast {
			content = content + p.thinkingTagClose
		}
		return content
	case ThinkingHandlingStripTags:
		// Remove tags but keep content
		return content
	case ThinkingHandlingAsReasoningContent:
		fallthrough
	default:
		return content
	}
}

// Finalize finalizes parsing and returns any remaining content
func (p *ThinkingParser) Finalize() *ThinkingParseResult {
	result := &ThinkingParseResult{}

	// If we have buffered content and never found thinking
	if p.buffer != "" && !p.foundThinking {
		result.RegularContent = p.buffer
		p.buffer = ""
	}

	// If we're still in thinking, close it
	if p.inThinking {
		result.ThinkingContent = p.processForOutput(p.thinkingContent, !p.firstThinkingSent, true)
		result.IsLastThinkingChunk = true
		p.inThinking = false
		p.thinkingEnded = true
	}

	return result
}

// FoundThinkingBlock returns whether a thinking block was found
func (p *ThinkingParser) FoundThinkingBlock() bool {
	return p.foundThinking
}
