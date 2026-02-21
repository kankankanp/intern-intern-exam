# セキュリティ設計

## 1. 概要

外部URLへアクセスするシステムのため、SSRF（Server-Side Request Forgery）対策を最重要課題として設計する。

## 2. SSRF対策

### 2.1 脅威モデル

```
┌─────────────────────────────────────────────────────────────────────┐
│                         攻撃シナリオ                                │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  攻撃者 ──▶ API ──▶ Worker ──▶ 内部サービス/メタデータエンドポイント  │
│                        │                                            │
│                        ▼                                            │
│               http://169.254.169.254/  (AWS メタデータ)            │
│               http://localhost:8080/admin                          │
│               http://192.168.1.100/internal-api                    │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

**リスク:**
- クラウドメタデータエンドポイントへのアクセス（認証情報漏洩）
- 内部ネットワークのスキャン
- 内部サービスへの不正アクセス

### 2.2 多層防御アプローチ

```
┌─────────────────────────────────────────────────────────────────────┐
│                        SSRF Guard                                   │
│                                                                     │
│  Layer 1: スキーム検証                                              │
│    └─▶ http/https のみ許可                                          │
│                                                                     │
│  Layer 2: ホスト検証（URL解析時）                                    │
│    └─▶ プライベートIPブロック                                        │
│    └─▶ localhost/loopbackブロック                                   │
│    └─▶ リンクローカルブロック                                        │
│                                                                     │
│  Layer 3: DNS解決後IP検証                                           │
│    └─▶ 解決IPがプライベートならブロック                              │
│    └─▶ DNS Rebinding対策                                            │
│                                                                     │
│  Layer 4: リダイレクト先検証                                         │
│    └─▶ 各リダイレクトでLayer 1-3を再検証                            │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.3 実装

```go
package httpclient

import (
    "errors"
    "net"
    "net/url"
    "strings"
)

var (
    ErrInvalidScheme    = errors.New("ssrf: invalid scheme, only http/https allowed")
    ErrPrivateIP        = errors.New("ssrf: private IP address not allowed")
    ErrLoopback         = errors.New("ssrf: loopback address not allowed")
    ErrLinkLocal        = errors.New("ssrf: link-local address not allowed")
    ErrInvalidHost      = errors.New("ssrf: invalid host")
)

type SSRFGuard struct {
    // 許可リスト（オプション）
    allowedHosts map[string]bool
}

func NewSSRFGuard() *SSRFGuard {
    return &SSRFGuard{
        allowedHosts: make(map[string]bool),
    }
}

// ValidateURL はURLの安全性を検証する
func (g *SSRFGuard) ValidateURL(rawURL string) error {
    u, err := url.Parse(rawURL)
    if err != nil {
        return err
    }

    // Layer 1: スキーム検証
    if err := g.validateScheme(u.Scheme); err != nil {
        return err
    }

    // Layer 2: ホスト検証
    if err := g.validateHost(u.Host); err != nil {
        return err
    }

    return nil
}

// ValidateIP はIPアドレスの安全性を検証する（DNS解決後に使用）
func (g *SSRFGuard) ValidateIP(ip net.IP) error {
    if ip == nil {
        return ErrInvalidHost
    }

    // ループバック (127.0.0.0/8, ::1)
    if ip.IsLoopback() {
        return ErrLoopback
    }

    // プライベートIP
    if ip.IsPrivate() {
        return ErrPrivateIP
    }

    // リンクローカル (169.254.0.0/16, fe80::/10)
    if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
        return ErrLinkLocal
    }

    // 未指定アドレス (0.0.0.0, ::)
    if ip.IsUnspecified() {
        return ErrPrivateIP
    }

    // 追加のプライベート範囲チェック
    if g.isBlockedRange(ip) {
        return ErrPrivateIP
    }

    return nil
}

func (g *SSRFGuard) validateScheme(scheme string) error {
    scheme = strings.ToLower(scheme)
    if scheme != "http" && scheme != "https" {
        return ErrInvalidScheme
    }
    return nil
}

func (g *SSRFGuard) validateHost(host string) error {
    // ポート分離
    hostname := host
    if h, _, err := net.SplitHostPort(host); err == nil {
        hostname = h
    }

    // 空ホスト
    if hostname == "" {
        return ErrInvalidHost
    }

    // localhost系
    if g.isLocalhost(hostname) {
        return ErrLoopback
    }

    // IPアドレス直接指定の場合
    if ip := net.ParseIP(hostname); ip != nil {
        return g.ValidateIP(ip)
    }

    return nil
}

func (g *SSRFGuard) isLocalhost(hostname string) bool {
    hostname = strings.ToLower(hostname)
    localNames := []string{
        "localhost",
        "localhost.localdomain",
        "127.0.0.1",
        "::1",
        "[::1]",
    }
    for _, name := range localNames {
        if hostname == name {
            return true
        }
    }
    return false
}

func (g *SSRFGuard) isBlockedRange(ip net.IP) bool {
    blockedRanges := []string{
        "0.0.0.0/8",       // Current network
        "10.0.0.0/8",      // Private (Class A)
        "100.64.0.0/10",   // Carrier-grade NAT
        "127.0.0.0/8",     // Loopback
        "169.254.0.0/16",  // Link-local
        "172.16.0.0/12",   // Private (Class B)
        "192.0.0.0/24",    // IETF Protocol Assignments
        "192.0.2.0/24",    // TEST-NET-1
        "192.168.0.0/16",  // Private (Class C)
        "198.18.0.0/15",   // Benchmarking
        "198.51.100.0/24", // TEST-NET-2
        "203.0.113.0/24",  // TEST-NET-3
        "224.0.0.0/4",     // Multicast
        "240.0.0.0/4",     // Reserved
        "255.255.255.255/32", // Broadcast

        // IPv6
        "::/128",          // Unspecified
        "::1/128",         // Loopback
        "fc00::/7",        // Unique local
        "fe80::/10",       // Link-local
        "ff00::/8",        // Multicast
    }

    for _, cidr := range blockedRanges {
        _, network, err := net.ParseCIDR(cidr)
        if err != nil {
            continue
        }
        if network.Contains(ip) {
            return true
        }
    }
    return false
}
```

### 2.4 DNS Rebinding対策

```go
// SafeDialer はDNS解決後のIP検証を行うDialer
type SafeDialer struct {
    guard   *SSRFGuard
    timeout time.Duration
}

func (d *SafeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
    host, port, err := net.SplitHostPort(addr)
    if err != nil {
        return nil, err
    }

    // DNS解決
    ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
    if err != nil {
        return nil, err
    }

    // 解決されたすべてのIPを検証
    var validIP net.IP
    for _, ip := range ips {
        if err := d.guard.ValidateIP(ip); err == nil {
            validIP = ip
            break
        }
    }

    if validIP == nil {
        return nil, ErrPrivateIP
    }

    // 検証済みIPで接続
    dialer := &net.Dialer{Timeout: d.timeout}
    return dialer.DialContext(ctx, network, net.JoinHostPort(validIP.String(), port))
}

// HTTPクライアント設定
func NewSafeHTTPClient(guard *SSRFGuard) *http.Client {
    return &http.Client{
        Transport: &http.Transport{
            DialContext: (&SafeDialer{
                guard:   guard,
                timeout: 10 * time.Second,
            }).DialContext,
        },
        Timeout: 30 * time.Second,
    }
}
```

### 2.5 リダイレクト検証

```go
func NewHTTPClientWithRedirectCheck(guard *SSRFGuard) *http.Client {
    return &http.Client{
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            // リダイレクト回数制限
            if len(via) >= 5 {
                return errors.New("too many redirects")
            }

            // リダイレクト先のSSRF検証
            if err := guard.ValidateURL(req.URL.String()); err != nil {
                return fmt.Errorf("redirect blocked: %w", err)
            }

            return nil
        },
    }
}
```

## 3. ブロック対象IP範囲一覧

| 範囲 | 用途 | 脅威 |
|------|------|------|
| `127.0.0.0/8` | ループバック | 内部サービスアクセス |
| `10.0.0.0/8` | プライベート(Class A) | 内部ネットワークアクセス |
| `172.16.0.0/12` | プライベート(Class B) | 内部ネットワークアクセス |
| `192.168.0.0/16` | プライベート(Class C) | 内部ネットワークアクセス |
| `169.254.0.0/16` | リンクローカル | AWSメタデータ等 |
| `100.64.0.0/10` | CGNAT | 共有ネットワーク |
| `::1/128` | IPv6ループバック | 内部サービスアクセス |
| `fc00::/7` | IPv6ユニークローカル | 内部ネットワークアクセス |
| `fe80::/10` | IPv6リンクローカル | リンクローカルサービス |

## 4. クラウドメタデータ対策

### 4.1 AWSメタデータサービス

```go
// AWSメタデータエンドポイントを明示的にブロック
var awsMetadataIPs = []string{
    "169.254.169.254",
    "fd00:ec2::254",  // IPv6
}

func (g *SSRFGuard) isAWSMetadata(host string) bool {
    for _, ip := range awsMetadataIPs {
        if host == ip {
            return true
        }
    }
    return false
}
```

### 4.2 GCPメタデータサービス

```go
var gcpMetadataHosts = []string{
    "metadata.google.internal",
    "169.254.169.254",
}
```

## 5. User-Agent設定

外部サイトへのアクセス時は明示的なUser-Agentを設定:

```go
const UserAgent = "InternInc-MetadataCollector/0.1 (+https://example.com/bot)"

func (f *Fetcher) Fetch(ctx context.Context, targetURL string) (*FetchResult, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("User-Agent", UserAgent)
    // ...
}
```

## 6. レスポンス制限

### 6.1 サイズ制限

メモリ枯渇攻撃対策:

```go
const MaxResponseSize = 1 * 1024 * 1024 // 1MB

func (f *Fetcher) readBody(resp *http.Response) ([]byte, error) {
    return io.ReadAll(io.LimitReader(resp.Body, MaxResponseSize))
}
```

### 6.2 タイムアウト

リソース占有攻撃対策:

```go
client := &http.Client{
    Timeout: 8 * time.Second,  // 全体タイムアウト
    Transport: &http.Transport{
        DialContext: (&net.Dialer{
            Timeout: 5 * time.Second,  // 接続タイムアウト
        }).DialContext,
        TLSHandshakeTimeout:   5 * time.Second,
        ResponseHeaderTimeout: 5 * time.Second,
    },
}
```

## 7. 入力バリデーション

### 7.1 URL登録時

```go
func ValidateURLForRegistration(rawURL string) error {
    u, err := url.Parse(rawURL)
    if err != nil {
        return fmt.Errorf("invalid URL format: %w", err)
    }

    // スキーム検証
    if u.Scheme != "http" && u.Scheme != "https" {
        return errors.New("only http and https schemes are allowed")
    }

    // ホストの存在確認
    if u.Host == "" {
        return errors.New("host is required")
    }

    // SSRF検証
    guard := NewSSRFGuard()
    if err := guard.ValidateURL(rawURL); err != nil {
        return fmt.Errorf("URL blocked for security reasons: %w", err)
    }

    return nil
}
```

### 7.2 SQLインジェクション対策

プリペアドステートメントを必ず使用:

```go
// 正しい例
stmt, err := db.PrepareContext(ctx, "SELECT * FROM urls WHERE id = $1")
row := stmt.QueryRowContext(ctx, urlID)

// 誤った例（絶対にしない）
// query := fmt.Sprintf("SELECT * FROM urls WHERE id = %s", urlID)
```

## 8. セキュリティチェックリスト

### 8.1 必須項目

- [x] http/httpsスキームのみ許可
- [x] プライベートIPブロック
- [x] ループバックアドレスブロック
- [x] リンクローカルアドレスブロック
- [x] DNS解決後のIP検証
- [x] リダイレクト先の再検証
- [x] レスポンスサイズ制限
- [x] リクエストタイムアウト
- [x] User-Agent明示
- [x] プリペアドステートメント使用

### 8.2 推奨項目

- [ ] 許可ホストのホワイトリスト機能
- [ ] リクエストログの監査
- [ ] 異常パターン検知（同一IPへの大量アクセス等）
- [ ] レート制限による過負荷保護

## 9. セキュリティテスト

### 9.1 テストケース

```go
func TestSSRFGuard_ValidateURL(t *testing.T) {
    guard := NewSSRFGuard()

    tests := []struct {
        name    string
        url     string
        wantErr bool
    }{
        // 許可されるべきURL
        {"valid https", "https://example.com", false},
        {"valid http", "http://example.com", false},
        {"valid with path", "https://example.com/page", false},

        // ブロックされるべきURL
        {"localhost", "http://localhost/", true},
        {"127.0.0.1", "http://127.0.0.1/", true},
        {"private 10.x", "http://10.0.0.1/", true},
        {"private 172.x", "http://172.16.0.1/", true},
        {"private 192.168.x", "http://192.168.1.1/", true},
        {"link-local", "http://169.254.169.254/", true},
        {"ftp scheme", "ftp://example.com/", true},
        {"file scheme", "file:///etc/passwd", true},
        {"ipv6 loopback", "http://[::1]/", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := guard.ValidateURL(tt.url)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
            }
        })
    }
}
```
