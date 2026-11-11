package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	pb "orchestra/api/proto/orchestra/v1"
)

var (
	ErrSSRFBlocked        = errors.New("request blocked by SSRF protection: target resolves to private/loopback/link-local address")
	ErrUnsupportedScheme  = errors.New("unsupported protocol scheme: only http and https are allowed")
	ErrResponseTooLarge   = errors.New("response body exceeded maximum allowed size")
	ErrHttpStatusMismatch = errors.New("unexpected HTTP response status")
	ErrEmptyURL           = errors.New("http task requires a non-empty url")
)

const (
	DefaultMaxResponseBytes = 2 * 1024 * 1024  // 2MB
	MaxAllowedResponseBytes = 10 * 1024 * 1024 // 10MB
)

// HTTPTaskPayload represents the parameters for executing an HTTP task.
type HTTPTaskPayload struct {
	URL              string            `json:"url"`
	Method           string            `json:"method,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Body             string            `json:"body,omitempty"`
	ExpectedStatus   int               `json:"expected_status,omitempty"`
	MaxResponseBytes int64             `json:"max_response_bytes,omitempty"`
	AllowPrivate     bool              `json:"allow_private,omitempty"`
}

// IsPrivateOrRestrictedIP returns true if the IP is loopback, link-local, RFC1918 private, multicast, or unspecified.
func IsPrivateOrRestrictedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}

	// IPv4 checks
	if ipv4 := ip.To4(); ipv4 != nil {
		// 10.0.0.0/8
		if ipv4[0] == 10 {
			return true
		}
		// 172.16.0.0/12 (172.16.x.x - 172.31.x.x)
		if ipv4[0] == 172 && ipv4[1] >= 16 && ipv4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ipv4[0] == 192 && ipv4[1] == 168 {
			return true
		}
		// Cloud metadata / link-local 169.254.0.0/16
		if ipv4[0] == 169 && ipv4[1] == 254 {
			return true
		}
		// 100.64.0.0/10 (Carrier grade NAT)
		if ipv4[0] == 100 && (ipv4[1]&0xC0) == 64 {
			return true
		}
		// 127.0.0.0/8
		if ipv4[0] == 127 {
			return true
		}
		// 0.0.0.0/8
		if ipv4[0] == 0 {
			return true
		}
		return false
	}

	// IPv6 Unique Local Addresses (fc00::/7)
	if len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc {
		return true
	}

	return false
}

// SafeDialer creates a net.Dialer that prevents SSRF by checking IP addresses during connection.
func SafeDialer(allowPrivate bool) *net.Dialer {
	return &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			ip := net.ParseIP(host)
			if ip != nil && IsPrivateOrRestrictedIP(ip) {
				return fmt.Errorf("%w: %s", ErrSSRFBlocked, ip.String())
			}
			return nil
		},
	}
}

// ExecuteHTTPTask executes an HTTP request according to payload specification with SSRF guards.
func ExecuteHTTPTask(ctx context.Context, payloadStr string) ([]byte, pb.TaskOutcome, string) {
	var payload HTTPTaskPayload
	trimmed := strings.TrimSpace(payloadStr)

	// Support both raw JSON object or plain URL string
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, fmt.Sprintf("invalid http payload JSON: %v", err)
		}
	} else if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		payload.URL = trimmed
		payload.Method = http.MethodGet
		payload.ExpectedStatus = http.StatusOK
	} else {
		return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, "http payload must be a JSON object or a valid http(s) URL"
	}

	if payload.URL == "" {
		return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, ErrEmptyURL.Error()
	}

	parsedURL, err := url.Parse(payload.URL)
	if err != nil {
		return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, fmt.Sprintf("invalid URL: %v", err)
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, ErrUnsupportedScheme.Error()
	}

	// Validate target hostname / IP for SSRF
	host := parsedURL.Hostname()
	if !payload.AllowPrivate {
		// If hostname is directly an IP
		if ip := net.ParseIP(host); ip != nil {
			if IsPrivateOrRestrictedIP(ip) {
				return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, fmt.Sprintf("%v: %s", ErrSSRFBlocked, ip.String())
			}
		} else {
			// Resolve hostname to check IPs before dial
			ips, resolveErr := net.LookupIP(host)
			if resolveErr == nil {
				for _, ip := range ips {
					if IsPrivateOrRestrictedIP(ip) {
						return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, fmt.Sprintf("%v: hostname '%s' resolved to restricted IP %s", ErrSSRFBlocked, host, ip.String())
					}
				}
			}
		}
	}

	method := strings.ToUpper(strings.TrimSpace(payload.Method))
	if method == "" {
		method = http.MethodGet
	}

	expectedStatus := payload.ExpectedStatus
	if expectedStatus <= 0 {
		expectedStatus = http.StatusOK
	}

	maxBytes := payload.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxResponseBytes
	} else if maxBytes > MaxAllowedResponseBytes {
		maxBytes = MaxAllowedResponseBytes
	}

	var bodyReader io.Reader
	if payload.Body != "" {
		bodyReader = strings.NewReader(payload.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, payload.URL, bodyReader)
	if err != nil {
		return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, fmt.Sprintf("failed to construct request: %v", err)
	}

	for k, v := range payload.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Orchestra-Worker/1.0")
	}

	dialer := SafeDialer(payload.AllowPrivate)
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   0, // context handles the timeout
	}

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, pb.TaskOutcome_TASK_OUTCOME_TIMEOUT, "http request timed out"
		}
		if errors.Is(err, ErrSSRFBlocked) || strings.Contains(err.Error(), ErrSSRFBlocked.Error()) {
			return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, fmt.Sprintf("%v: %v", ErrSSRFBlocked, err)
		}
		return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, fmt.Sprintf("http request failed: %v", err)
	}
	defer resp.Body.Close()

	limitReader := io.LimitReader(resp.Body, maxBytes+1)
	respData, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, fmt.Sprintf("failed to read response body: %v", err)
	}

	if int64(len(respData)) > maxBytes {
		return nil, pb.TaskOutcome_TASK_OUTCOME_FAILURE, fmt.Sprintf("%v: limit is %d bytes", ErrResponseTooLarge, maxBytes)
	}

	if resp.StatusCode != expectedStatus {
		preview := string(respData)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return respData, pb.TaskOutcome_TASK_OUTCOME_FAILURE, fmt.Sprintf("%v: expected %d, got %d. Body: %s", ErrHttpStatusMismatch, expectedStatus, resp.StatusCode, preview)
	}

	return respData, pb.TaskOutcome_TASK_OUTCOME_SUCCESS, ""
}
