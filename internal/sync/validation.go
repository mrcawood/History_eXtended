package sync

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Security validation errors
var (
	ErrInvalidIdentifierType   = errors.New("invalid identifier type")
	ErrInvalidIdentifierLength = errors.New("invalid identifier length")
	ErrInvalidIdentifierFormat = errors.New("invalid identifier format")
	ErrPathTraversalAttempt    = errors.New("path traversal attempt detected")
)

// Valid identifier patterns for security
var (
	// VaultID: alphanumeric with hyphens, underscores, dots (3-64 chars)
	vaultIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,64}$`)

	// NodeID: alphanumeric with hyphens, underscores (3-64 chars)
	nodeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,64}$`)

	// SegmentID: hex hash (64 chars) or UUID format
	segmentIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$|^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// TombstoneID: similar to segmentID
	tombstoneIDPattern = segmentIDPattern
)

// ValidateIdentifier checks if an identifier meets security requirements
func ValidateIdentifier(id, idType string) error {
	var pattern *regexp.Regexp
	var minLen, maxLen int

	// Check for path traversal attempts FIRST (before length check)
	if strings.Contains(id, "..") || strings.Contains(id, "\x00") {
		return ErrPathTraversalAttempt
	}

	switch strings.ToLower(idType) {
	case "vault":
		pattern = vaultIDPattern
		minLen, maxLen = 3, 64
	case "node":
		pattern = nodeIDPattern
		minLen, maxLen = 3, 64
	case "segment":
		pattern = segmentIDPattern
		minLen, maxLen = 32, 64 // Allow UUID length
	case "tombstone":
		pattern = tombstoneIDPattern
		minLen, maxLen = 32, 64
	default:
		return ErrInvalidIdentifierType
	}

	if len(id) < minLen || len(id) > maxLen {
		return ErrInvalidIdentifierLength
	}

	if !pattern.MatchString(id) {
		return ErrInvalidIdentifierFormat
	}

	return nil
}

// SanitizePath ensures path components are safe
func SanitizePath(components ...string) string {
	safe := make([]string, 0, len(components))
	for _, comp := range components {
		// Remove any path separators and null bytes
		clean := strings.ReplaceAll(comp, "/", "_")
		clean = strings.ReplaceAll(clean, "\\", "_")
		clean = strings.ReplaceAll(clean, "\x00", "_")
		safe = append(safe, clean)
	}
	return strings.Join(safe, "/")
}

// SecureSegmentKey creates a safe segment key with validation
func SecureSegmentKey(vaultID, nodeID, segmentID string) (string, error) {
	// Validate all components
	if err := ValidateIdentifier(vaultID, "vault"); err != nil {
		return "", fmt.Errorf("invalid vault ID: %w", err)
	}
	if err := ValidateIdentifier(nodeID, "node"); err != nil {
		return "", fmt.Errorf("invalid node ID: %w", err)
	}
	if err := ValidateIdentifier(segmentID, "segment"); err != nil {
		return "", fmt.Errorf("invalid segment ID: %w", err)
	}

	// Construct safe key
	return fmt.Sprintf("vaults/%s/objects/segments/%s/%s.hxseg",
		vaultID, nodeID, segmentID), nil
}

// SecureTombstoneKey creates a safe tombstone key with validation
func SecureTombstoneKey(vaultID, nodeID, tombstoneID string) (string, error) {
	// Validate all components
	if err := ValidateIdentifier(vaultID, "vault"); err != nil {
		return "", fmt.Errorf("invalid vault ID: %w", err)
	}
	if err := ValidateIdentifier(nodeID, "node"); err != nil {
		return "", fmt.Errorf("invalid node ID: %w", err)
	}
	if err := ValidateIdentifier(tombstoneID, "tombstone"); err != nil {
		return "", fmt.Errorf("invalid tombstone ID: %w", err)
	}

	// Construct safe key
	return fmt.Sprintf("vaults/%s/objects/tombstones/%s/%s.hxtomb",
		vaultID, nodeID, tombstoneID), nil
}

// SecureManifestKey creates a safe manifest key with validation
func SecureManifestKey(vaultID, nodeID string) (string, error) {
	// Validate all components
	if err := ValidateIdentifier(vaultID, "vault"); err != nil {
		return "", fmt.Errorf("invalid vault ID: %w", err)
	}
	if err := ValidateIdentifier(nodeID, "node"); err != nil {
		return "", fmt.Errorf("invalid node ID: %w", err)
	}

	// Construct safe key
	return fmt.Sprintf("vaults/%s/objects/manifests/%s.hxman", vaultID, nodeID), nil
}
