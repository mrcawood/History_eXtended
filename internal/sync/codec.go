package sync

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

// Object wire format: 4-byte big-endian header length | header JSON | body (encrypted or plain)

// EncodeSegment builds a full .hxseg object: header (JSON) + encrypted payload.
// If encrypt is false (trusted store), payload is plaintext; CryptoEnv is empty.
func EncodeSegment(h *Header, payload *SegmentPayload, K_master []byte, encrypt bool) ([]byte, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return encodeObject(h, payloadJSON, K_master, encrypt)
}

// EncodeBlob builds a full .hxblob object.
func EncodeBlob(h *Header, plaintext []byte, K_master []byte, encrypt bool) ([]byte, error) {
	return encodeObject(h, plaintext, K_master, encrypt)
}

// EncodeTombstone builds a full .hxtomb object.
func EncodeTombstone(h *Header, payload *TombstonePayload, K_master []byte, encrypt bool) ([]byte, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return encodeObject(h, payloadJSON, K_master, encrypt)
}

func encodeObject(h *Header, plaintext []byte, K_master []byte, encrypt bool) ([]byte, error) {
	var body []byte

	if encrypt {
		if len(K_master) != KeySize {
			return nil, fmt.Errorf("K_master must be %d bytes", KeySize)
		}
		K_obj := make([]byte, KeySize)
		if _, err := io.ReadFull(rand.Reader, K_obj); err != nil {
			return nil, err
		}
		nonce := make([]byte, NonceSize)
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return nil, err
		}
		wrapped, err := WrapKey(K_master, K_obj)
		if err != nil {
			return nil, err
		}
		h.Crypto = CryptoEnv{
			NonceHex:   hex.EncodeToString(nonce),
			WrappedKey: hex.EncodeToString(wrapped),
		}
		headerBytes, err := json.Marshal(h)
		if err != nil {
			return nil, err
		}
		ct, err := SealWithKey(K_obj, nonce, plaintext, headerBytes)
		if err != nil {
			return nil, err
		}
		body = ct
		return marshalObject(headerBytes, body), nil
	}

	h.Crypto = CryptoEnv{}
	headerBytes, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	return marshalObject(headerBytes, plaintext), nil
}

func marshalObject(header, body []byte) []byte {
	buf := make([]byte, 4, 4+len(header)+len(body))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(header)))
	buf = append(buf, header...)
	return append(buf, body...)
}

// DecodeObject parses object bytes, returns header and body. Does not decrypt.
func DecodeObject(raw []byte) (*Header, []byte, error) {
	if len(raw) < 4 {
		return nil, nil, fmt.Errorf("object too short")
	}
	headerLen := binary.BigEndian.Uint32(raw[:4])
	if headerLen > 1024*1024 {
		return nil, nil, fmt.Errorf("header too long")
	}
	if len(raw) < 4+int(headerLen) {
		return nil, nil, fmt.Errorf("truncated object")
	}
	headerBytes := raw[4 : 4+headerLen]
	body := raw[4+headerLen:]

	var h Header
	if err := json.Unmarshal(headerBytes, &h); err != nil {
		return nil, nil, fmt.Errorf("parse header: %w", err)
	}
	if h.Magic != Magic || h.Version != Version {
		return nil, nil, fmt.Errorf("invalid magic/version")
	}
	return &h, body, nil
}

// DecryptObject decrypts body using K_master and header. Returns plaintext.
func DecryptObject(h *Header, body []byte, K_master []byte) ([]byte, error) {
	if h.Crypto.NonceHex == "" || h.Crypto.WrappedKey == "" {
		return body, nil
	}
	headerBytes, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	nonce, err := hex.DecodeString(h.Crypto.NonceHex)
	if err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	wrapped, err := hex.DecodeString(h.Crypto.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("wrapped key: %w", err)
	}
	return DecryptPayload(K_master, nonce, body, wrapped, headerBytes)
}
