package s3compat

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"mework/libs/shared/core"
)

// DefaultS3Endpoint is the default AWS S3 endpoint.
const DefaultS3Endpoint = "https://s3.amazonaws.com"

// Client is a minimal S3-compatible HTTP client that speaks the S3 REST API.
type Client struct {
	endpoint   string
	region     string
	bucket     string
	signer     *Signer
	httpClient *http.Client
}

// Config configures an S3-compatible client.
type Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

// NewClient creates a new S3-compatible client.
func NewClient(cfg Config) *Client {
	ep := cfg.Endpoint
	if ep == "" {
		ep = DefaultS3Endpoint
	}
	return &Client{
		endpoint:   strings.TrimRight(ep, "/"),
		region:     cfg.Region,
		bucket:     cfg.Bucket,
		signer:     NewSigner(cfg.AccessKey, cfg.SecretKey, cfg.Region),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// objectURL builds the full URL for an object in the bucket.
func (c *Client) objectURL(key string) string {
	return fmt.Sprintf("%s/%s/%s", c.endpoint, c.bucket, path.Clean(key))
}

// bucketURL builds the URL for the bucket (used for listing).
func (c *Client) bucketURL() string {
	return fmt.Sprintf("%s/%s", c.endpoint, c.bucket)
}

// doSignedRequest signs and executes an HTTP request.
func (c *Client) doSignedRequest(ctx context.Context, method, urlStr string, body io.Reader, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	t := time.Now().UTC()
	if body != nil {
		// Read body to compute hash; for the S3 API we hash the payload.
		// For simplicity, we read the body into a buffer and replace it.
		buf, err := io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("reading body for signing: %w", err)
		}
		payloadHash := sha256Hex(buf)
		req.Header.Set("x-amz-content-sha256", payloadHash)
		req.Body = io.NopCloser(bytes.NewReader(buf))
		req.ContentLength = int64(len(buf))

		// Build header map for signing.
		h := make(map[string]string)
		for k, v := range req.Header {
			h[strings.ToLower(k)] = v[0]
		}
		auth := c.signer.AuthorizationHeader(method, req.URL.Path, req.URL.Query(), h, payloadHash, t)
		req.Header.Set("Authorization", auth)
	} else {
		req.Header.Set("x-amz-content-sha256", unsignedPayload)

		h := make(map[string]string)
		for k, v := range req.Header {
			h[strings.ToLower(k)] = v[0]
		}
		auth := c.signer.AuthorizationHeader(method, req.URL.Path, req.URL.Query(), h, unsignedPayload, t)
		req.Header.Set("Authorization", auth)
	}

	return c.httpClient.Do(req)
}

// PutObject stores an object. The reader is consumed entirely.
func (c *Client) PutObject(ctx context.Context, key string, reader io.Reader) (string, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("reading body: %w", err)
	}

	etag := md5Hex(body)
	headers := map[string]string{
		"Content-Type":   "application/octet-stream",
		"Content-Length": strconv.Itoa(len(body)),
	}

	resp, err := c.doSignedRequest(ctx, http.MethodPut, c.objectURL(key), bytes.NewReader(body), headers)
	if err != nil {
		return "", fmt.Errorf("put object %s: %w", key, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", s3Error(resp, "put object")
	}

	// Return ETag from response if available, otherwise our computed one.
	if respETag := resp.Header.Get("ETag"); respETag != "" {
		return strings.Trim(respETag, "\""), nil
	}
	return etag, nil
}

// GetObject retrieves an object's contents.
func (c *Client) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := c.doSignedRequest(ctx, http.MethodGet, c.objectURL(key), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", key, err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, core.ObjectDeleted
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, s3Error(resp, "get object")
	}

	return resp.Body, nil
}

// HeadObject returns metadata for an object.
func (c *Client) HeadObject(ctx context.Context, key string) (core.ObjectInfo, error) {
	resp, err := c.doSignedRequest(ctx, http.MethodHead, c.objectURL(key), nil, nil)
	if err != nil {
		return core.ObjectInfo{}, fmt.Errorf("head object %s: %w", key, err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return core.ObjectInfo{}, core.ObjectDeleted
	}

	if resp.StatusCode != http.StatusOK {
		return core.ObjectInfo{}, s3Error(resp, "head object")
	}

	info := core.ObjectInfo{
		Ref:  core.ObjectRef{Bucket: c.bucket, Key: key},
		Size: parseContentLength(resp.Header.Get("Content-Length")),
		ETag: strings.Trim(resp.Header.Get("ETag"), "\""),
	}

	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if t, err := time.Parse(http.TimeFormat, lm); err == nil {
			info.LastModified = t
		}
	}

	return info, nil
}

// DeleteObject removes an object.
func (c *Client) DeleteObject(ctx context.Context, key string) error {
	resp, err := c.doSignedRequest(ctx, http.MethodDelete, c.objectURL(key), nil, nil)
	if err != nil {
		return fmt.Errorf("delete object %s: %w", key, err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return s3Error(resp, "delete object")
}

// PartResult holds the part number and ETag for a completed multipart part.
type PartResult struct {
	PartNumber int
	ETag       string
}

// ListBucketResult is the XML response for ListObjectsV2.
type ListBucketResult struct {
	XMLName       xml.Name        `xml:"ListBucketResult"`
	Contents      []ListEntry     `xml:"Contents"`
	IsTruncated   bool            `xml:"IsTruncated"`
	NextToken     string          `xml:"NextContinuationToken"`
}

// ListEntry is a single object entry in the listing response.
type ListEntry struct {
	Key          string `xml:"Key"`
	Size         int64  `xml:"Size"`
	ETag         string `xml:"ETag"`
	LastModified string `xml:"LastModified"`
}

// ListObjects returns objects whose key begins with the given prefix.
func (c *Client) ListObjects(ctx context.Context, prefix string) ([]core.ObjectInfo, error) {
	u, err := url.Parse(c.bucketURL())
	if err != nil {
		return nil, fmt.Errorf("parsing bucket URL: %w", err)
	}

	q := url.Values{}
	q.Set("list-type", "2")
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	u.RawQuery = q.Encode()

	resp, err := c.doSignedRequest(ctx, http.MethodGet, u.String(), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, s3Error(resp, "list objects")
	}

	var result ListBucketResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding list response: %w", err)
	}

	objects := make([]core.ObjectInfo, 0, len(result.Contents))
	for _, entry := range result.Contents {
		info := core.ObjectInfo{
			Ref:  core.ObjectRef{Bucket: c.bucket, Key: entry.Key},
			Size: entry.Size,
			ETag: strings.Trim(entry.ETag, "\""),
		}
		if entry.LastModified != "" {
			if t, err := time.Parse(time.RFC3339, entry.LastModified); err == nil {
				info.LastModified = t
			}
		}
		objects = append(objects, info)
	}

	return objects, nil
}

// PresignGetURL mints a presigned GET URL.
func (c *Client) PresignGetURL(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	expires := time.Now().UTC().Add(ttl)
	headers := map[string]string{"host": c.host()}
	q := c.signer.PresignQuery(http.MethodGet, "/"+c.bucket+"/"+key, headers, expires, ttl)

	u := fmt.Sprintf("%s/%s/%s?%s", c.endpoint, c.bucket, key, q.Encode())
	return u, expires, nil
}

// PresignPutURL mints a presigned PUT URL.
func (c *Client) PresignPutURL(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	expires := time.Now().UTC().Add(ttl)
	headers := map[string]string{"host": c.host()}
	q := c.signer.PresignQuery(http.MethodPut, "/"+c.bucket+"/"+key, headers, expires, ttl)

	u := fmt.Sprintf("%s/%s/%s?%s", c.endpoint, c.bucket, key, q.Encode())
	return u, expires, nil
}

// PutMultipart uploads a large object as ordered parts. It uses the S3
// Multipart Upload API: initiate, upload each part, then complete.
func (c *Client) PutMultipart(ctx context.Context, key string, parts []io.Reader) (string, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("no parts provided")
	}

	// Step 1: Initiate multipart upload.
	uploadID, err := c.initiateMultipart(ctx, key)
	if err != nil {
		return "", fmt.Errorf("initiating multipart: %w", err)
	}

	// Step 2: Upload each part.
	var partResults []PartResult

	for i, part := range parts {
		partNum := i + 1
		body, err := io.ReadAll(part)
		if err != nil {
			// Abort on failure.
			_ = c.abortMultipart(ctx, key, uploadID)
			return "", fmt.Errorf("reading part %d: %w", partNum, err)
		}

		etag, err := c.uploadPart(ctx, key, uploadID, partNum, body)
		if err != nil {
			_ = c.abortMultipart(ctx, key, uploadID)
			return "", fmt.Errorf("uploading part %d: %w", partNum, err)
		}
		partResults = append(partResults, PartResult{PartNumber: partNum, ETag: etag})
	}

	// Step 3: Complete multipart upload.
	return c.completeMultipart(ctx, key, uploadID, partResults)
}

// initiateMultipart starts a multipart upload and returns the UploadId.
func (c *Client) initiateMultipart(ctx context.Context, key string) (string, error) {
	u := c.objectURL(key) + "?uploads"
	resp, err := c.doSignedRequest(ctx, http.MethodPost, u, nil, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", s3Error(resp, "initiate multipart upload")
	}

	var initResp struct {
		XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
		UploadID string   `xml:"UploadId"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&initResp); err != nil {
		return "", fmt.Errorf("decoding initiate response: %w", err)
	}
	return initResp.UploadID, nil
}

// uploadPart uploads a single part and returns its ETag.
func (c *Client) uploadPart(ctx context.Context, key, uploadID string, partNumber int, body []byte) (string, error) {
	u := fmt.Sprintf("%s?partNumber=%d&uploadId=%s", c.objectURL(key), partNumber, url.QueryEscape(uploadID))
	headers := map[string]string{
		"Content-Length": strconv.Itoa(len(body)),
	}

	resp, err := c.doSignedRequest(ctx, http.MethodPut, u, bytes.NewReader(body), headers)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", s3Error(resp, fmt.Sprintf("upload part %d", partNumber))
	}

	return strings.Trim(resp.Header.Get("ETag"), "\""), nil
}

// completeMultipart finalizes the multipart upload with all parts.
func (c *Client) completeMultipart(ctx context.Context, key, uploadID string, parts []PartResult) (string, error) {
	u := fmt.Sprintf("%s?uploadId=%s", c.objectURL(key), url.QueryEscape(uploadID))

	// Build the CompleteMultipartUpload XML.
	type completedPart struct {
		PartNumber int    `xml:"PartNumber"`
		ETag       string `xml:"ETag"`
	}
	type completeUpload struct {
		XMLName xml.Name        `xml:"CompleteMultipartUpload"`
		Parts   []completedPart `xml:"Part"`
	}

	compParts := make([]completedPart, len(parts))
	for i, p := range parts {
		compParts[i] = completedPart{PartNumber: p.PartNumber, ETag: fmt.Sprintf(`"%s"`, p.ETag)}
	}

	body, err := xml.Marshal(&completeUpload{Parts: compParts})
	if err != nil {
		return "", fmt.Errorf("marshaling complete request: %w", err)
	}

	headers := map[string]string{
		"Content-Type":   "application/xml",
		"Content-Length": strconv.Itoa(len(body)),
	}

	resp, err := c.doSignedRequest(ctx, http.MethodPost, u, bytes.NewReader(body), headers)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", s3Error(resp, "complete multipart upload")
	}

	var completeResp struct {
		XMLName xml.Name `xml:"CompleteMultipartUploadResult"`
		ETag    string   `xml:"ETag"`
		Key     string   `xml:"Key"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&completeResp); err != nil {
		return "", fmt.Errorf("decoding complete response: %w", err)
	}

	return strings.Trim(completeResp.ETag, "\""), nil
}

// abortMultipart aborts a multipart upload.
func (c *Client) abortMultipart(ctx context.Context, key, uploadID string) error {
	u := fmt.Sprintf("%s?uploadId=%s", c.objectURL(key), url.QueryEscape(uploadID))
	resp, err := c.doSignedRequest(ctx, http.MethodDelete, u, nil, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// host extracts the host from the endpoint URL.
func (c *Client) host() string {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return "s3.amazonaws.com"
	}
	return u.Host
}

// s3Error builds an error from an S3 error response.
func s3Error(resp *http.Response, op string) error {
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("s3 %s: status=%d body=%s", op, resp.StatusCode, string(body))
}

func parseContentLength(s string) int64 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func md5Hex(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}
