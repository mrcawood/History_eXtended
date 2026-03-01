package sync

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateIdentifier tests the identifier validation function
func TestValidateIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		idType  string
		wantErr bool
		errType error
	}{
		// Valid vault IDs
		{"Valid vault ID simple", "vault1", "vault", false, nil},
		{"Valid vault ID with hyphen", "my-vault", "vault", false, nil},
		{"Valid vault ID with underscore", "my_vault", "vault", false, nil},
		{"Valid vault ID with dots", "my.vault.test", "vault", false, nil},
		{"Valid vault ID max length", strings.Repeat("a", 64), "vault", false, nil},

		// Valid node IDs
		{"Valid node ID simple", "node1", "node", false, nil},
		{"Valid node ID complex", "node-01_prod", "node", false, nil},

		// Valid segment IDs (64-char hex)
		{"Valid segment ID hex", strings.Repeat("a", 64), "segment", false, nil},
		{"Valid segment ID UUID", "550e8400-e29b-41d4-a716-446655440000", "segment", false, nil},

		// Invalid lengths
		{"Too short vault ID", "ab", "vault", true, ErrInvalidIdentifierLength},
		{"Too long vault ID", strings.Repeat("a", 65), "vault", true, ErrInvalidIdentifierLength},

		// Invalid formats
		{"Invalid vault ID with slash", "vault/1", "vault", true, ErrInvalidIdentifierFormat},
		{"Invalid vault ID with space", "vault 1", "vault", true, ErrInvalidIdentifierFormat},
		{"Invalid segment ID non-hex", "not-a-hash", "segment", true, ErrInvalidIdentifierLength}, // "not-a-hash" is 9 chars, below 32 min

		// Path traversal attempts
		{"Path traversal with dots", "../../../etc/passwd", "vault", true, ErrPathTraversalAttempt},
		{"Path traversal with null", "vault\x00", "vault", true, ErrPathTraversalAttempt},

		// Invalid type
		{"Invalid identifier type", "test", "invalid", true, ErrInvalidIdentifierType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdentifier(tt.id, tt.idType)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.errType), "Expected error type %v, got %v", tt.errType, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSecureSegmentKey tests secure segment key generation
func TestSecureSegmentKey(t *testing.T) {
	tests := []struct {
		name      string
		vaultID   string
		nodeID    string
		segmentID string
		wantErr   bool
		wantKey   string
	}{
		{
			name:      "Valid inputs",
			vaultID:   "test-vault",
			nodeID:    "node-01",
			segmentID: strings.Repeat("a", 64),
			wantErr:   false,
			wantKey:   "vaults/test-vault/objects/segments/node-01/" + strings.Repeat("a", 64) + ".hxseg",
		},
		{
			name:      "Invalid vault ID",
			vaultID:   "../invalid",
			nodeID:    "node-01",
			segmentID: strings.Repeat("a", 64),
			wantErr:   true,
		},
		{
			name:      "Invalid node ID",
			vaultID:   "test-vault",
			nodeID:    "node/01",
			segmentID: strings.Repeat("a", 64),
			wantErr:   true,
		},
		{
			name:      "Invalid segment ID",
			vaultID:   "test-vault",
			nodeID:    "node-01",
			segmentID: "invalid",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := SecureSegmentKey(tt.vaultID, tt.nodeID, tt.segmentID)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantKey, key)
			}
		})
	}
}

// TestSecureTombstoneKey tests secure tombstone key generation
func TestSecureTombstoneKey(t *testing.T) {
	validVault := "test-vault"
	validNode := "node-01"
	validTombstone := strings.Repeat("b", 64)

	key, err := SecureTombstoneKey(validVault, validNode, validTombstone)
	require.NoError(t, err)
	assert.Equal(t, "vaults/test-vault/objects/tombstones/node-01/"+strings.Repeat("b", 64)+".hxtomb", key)
}

// TestSecureManifestKey tests secure manifest key generation
func TestSecureManifestKey(t *testing.T) {
	validVault := "test-vault"
	validNode := "node-01"

	key, err := SecureManifestKey(validVault, validNode)
	require.NoError(t, err)
	assert.Equal(t, "vaults/test-vault/objects/manifests/node-01.hxman", key)
}

// TestSanitizePath tests path sanitization
func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name       string
		components []string
		want       string
	}{
		{
			name:       "Normal components",
			components: []string{"vault1", "node1", "segment1"},
			want:       "vault1/node1/segment1",
		},
		{
			name:       "Components with slashes",
			components: []string{"vault/1", "node/1", "segment/1"},
			want:       "vault_1/node_1/segment_1",
		},
		{
			name:       "Components with backslashes",
			components: []string{"vault\\1", "node\\1", "segment\\1"},
			want:       "vault_1/node_1/segment_1",
		},
		{
			name:       "Components with null bytes",
			components: []string{"vault\x00", "node\x00", "segment\x00"},
			want:       "vault_/node_/segment_",
		},
		{
			name:       "Empty components",
			components: []string{},
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizePath(tt.components...)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestPathTraversalPrevention is a property-based test for path traversal
func TestPathTraversalPrevention(t *testing.T) {
	maliciousInputs := []string{
		"../../../etc/passwd",
		"..\\..\\windows\\system32",
		"\x00\x01\x02malicious",
		"normal/../../../escape",
		"vault/../../secret",
		"node\\..\\..\\admin",
		"segment\x00../../config",
		"./hidden",
		".hidden/file",
		"file/./hidden",
	}

	for _, input := range maliciousInputs {
		t.Run("Malicious input: "+input, func(t *testing.T) {
			// Test ValidateIdentifier
			err := ValidateIdentifier(input, "vault")
			assert.Error(t, err, "Should reject malicious input: %s", input)
			assert.True(t, errors.Is(err, ErrPathTraversalAttempt) || errors.Is(err, ErrInvalidIdentifierFormat))

			// Test SecureSegmentKey (should fail validation)
			_, err = SecureSegmentKey(input, "valid-node", strings.Repeat("a", 64))
			assert.Error(t, err)
		})
	}
}
