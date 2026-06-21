// Package s3compat implements AWS Signature V4 signing and a minimal
// S3-compatible HTTP client using only the Go standard library. It is
// shared by the s3, minio, and r2 drivers so that each driver links only
// its own package and needs no external SDK.
package s3compat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	awsAlgorithm    = "AWS4-HMAC-SHA256"
	awsService      = "s3"
	unsignedPayload = "UNSIGNED-PAYLOAD"
	iso8601Basic    = "20060102T150405Z"
	iso8601Date     = "20060102"
)

// Signer computes AWS Signature V4 signatures.
type Signer struct {
	accessKey string
	secretKey string
	region    string
}

// NewSigner creates a new SigV4 signer.
func NewSigner(accessKey, secretKey, region string) *Signer {
	return &Signer{
		accessKey: accessKey,
		secretKey: secretKey,
		region:    region,
	}
}

// CredentialScope returns the slash-joined scope string for the given time.
func (s *Signer) CredentialScope(t time.Time) string {
	return fmt.Sprintf("%s/%s/%s/aws4_request",
		t.UTC().Format(iso8601Date),
		s.region,
		awsService,
	)
}

// AuthorizationHeader builds the full Authorization header for a request.
func (s *Signer) AuthorizationHeader(method, uri string, query url.Values, headers map[string]string, bodyHash string, t time.Time) string {
	credScope := s.CredentialScope(t)
	hdrStr, signedHeaders := canonicalHeaders(headers)
	canonicalReq := canonicalRequest(method, uri, canonicalQueryString(query), hdrStr, signedHeaders, bodyHash)
	stringToSign := buildStringToSign(t, credScope, canonicalReq)
	signingKey := s.signingKey(t)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	return fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		awsAlgorithm, s.accessKey, credScope, signedHeaders, signature)
}

// PresignQuery builds the query parameters for a presigned URL.
func (s *Signer) PresignQuery(method, uri string, headers map[string]string, t time.Time, expires time.Duration) url.Values {
	credScope := s.CredentialScope(t)
	_, signedHeaders := canonicalHeaders(headers)

	q := url.Values{}
	q.Set("X-Amz-Algorithm", awsAlgorithm)
	q.Set("X-Amz-Credential", fmt.Sprintf("%s/%s", s.accessKey, credScope))
	q.Set("X-Amz-Date", t.UTC().Format(iso8601Basic))
	q.Set("X-Amz-Expires", fmt.Sprintf("%d", int(expires.Seconds())))
	q.Set("X-Amz-SignedHeaders", signedHeaders)

	// Build canonical request with the presigned query params (minus signature).
	presignQ := make(url.Values)
	for k, v := range q {
		presignQ[k] = v
	}
	hdrStr, _ := canonicalHeaders(headers)
	presignCanonicalQ := canonicalQueryString(presignQ)
	canonicalReq := canonicalRequest(method, uri, presignCanonicalQ, hdrStr, signedHeaders, unsignedPayload)
	stringToSign := buildStringToSign(t, credScope, canonicalReq)
	signingKey := s.signingKey(t)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	q.Set("X-Amz-Signature", signature)
	return q
}

func (s *Signer) signingKey(t time.Time) []byte {
	dateKey := hmacSHA256([]byte("AWS4"+s.secretKey), t.UTC().Format(iso8601Date))
	dateRegionKey := hmacSHA256(dateKey, s.region)
	dateRegionServiceKey := hmacSHA256(dateRegionKey, awsService)
	return hmacSHA256(dateRegionServiceKey, "aws4_request")
}

func hmacSHA256(key []byte, msg string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msg))
	return mac.Sum(nil)
}

func hashSHA256(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

func canonicalRequest(method, uri, canonicalQuery, canonicalHeaders, signedHeaders, payloadHash string) string {
	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		method, uri, canonicalQuery, canonicalHeaders, signedHeaders, payloadHash)
}

func buildStringToSign(t time.Time, credScope, canonicalReq string) string {
	return fmt.Sprintf("%s\n%s\n%s\n%s",
		awsAlgorithm,
		t.UTC().Format(iso8601Basic),
		credScope,
		hashSHA256(canonicalReq),
	)
}

// canonicalHeaders builds the canonical header string and the signed headers list.
func canonicalHeaders(headers map[string]string) (string, string) {
	if len(headers) == 0 {
		return "", ""
	}

	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, strings.ToLower(k))
	}
	sort.Strings(keys)

	var sb strings.Builder
	var signed []string
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte(':')
		sb.WriteString(strings.TrimSpace(headers[k]))
		sb.WriteByte('\n')
		signed = append(signed, k)
	}
	return sb.String(), strings.Join(signed, ";")
}

// canonicalQueryString builds the canonical query string from url.Values.
func canonicalQueryString(q url.Values) string {
	if len(q) == 0 {
		return ""
	}

	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(url.QueryEscape(k))
		sb.WriteByte('=')
		if len(q[k]) > 0 {
			sb.WriteString(url.QueryEscape(q[k][0]))
		}
	}
	return sb.String()
}
