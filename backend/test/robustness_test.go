// 本文件收录第四批修复的回归用例：限流器原子性与内存上界、JWT 算法/issuer 校验、
// 分页 OFFSET 上界、角色聚合查询的错误传播。
//
// 与 hardening_test.go 的分工：那里是「缺陷已修好之后验证业务不受影响」，
// 这里直接针对**被改动的机制本身**——限流器并发、JWT 验签、分页边界都是
// 「不改测试就可能悄悄退化」的地方，必须有专门用例钉住。
package test

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	// 本文件同时用到「上游的 golang-jwt 库」与「本项目的 pkg/jwt」：
	// 前者只用来手工构造畸形令牌（算法不符 / issuer 不符 / uid=0），
	// 后者（被测对象）负责验签。两者包名都叫 jwt，故给上游起别名 upstream。
	upstream "github.com/golang-jwt/jwt/v5"

	"nova-erp/internal/config"
	"nova-erp/internal/pkg/jwt"
	"nova-erp/internal/pkg/limiter"
	"nova-erp/internal/pkg/page"
)

// ---------------------------------------------------------------------------
// 1. 限流器：AllowAndConsume 的并发原子性
// ---------------------------------------------------------------------------

// TestLimiterAllowAndConsumeIsAtomic 验证并发下阈值不会被突破。
//
// 旧写法是 Allow() + Fail() 两步调用：100 个 goroutine 同时进入时，
// 它们可能**全部**读到 fails=4（还没达到 max=5），然后全部 Fail，
// 最终计数变成 104 —— 限流器恰好在爆破场景下失效。
// 这里断言：100 个并发请求里，通过的必须恰好是 max 个。
func TestLimiterAllowAndConsumeIsAtomic(t *testing.T) {
	const max = 5
	l := limiter.New(max, time.Minute)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)
	start := make(chan struct{})
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // 让 100 个 goroutine 尽量同时冲进临界区
			if l.AllowAndConsume("same-key") {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if allowed != max {
		t.Fatalf("并发下放行数应恰好等于阈值 max=%d，实际 %d —— 原子性被破坏", max, allowed)
	}

	keys, evicted := l.Stats()
	if keys != 1 {
		t.Fatalf("应只有 1 个 key，实际 %d", keys)
	}
	if evicted != 0 {
		t.Fatalf("未超上界时不应有淘汰，实际 %d", evicted)
	}
}

// TestLimiterConcurrentDifferentKeysIndependent 确认不同 key 之间不互相干扰。
func TestLimiterConcurrentDifferentKeysIndependent(t *testing.T) {
	l := limiter.New(2, time.Minute)

	var wg sync.WaitGroup
	var mu sync.Mutex
	pass := make(map[string]int)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("10.0.0.%d", n)
			if l.AllowAndConsume(key) {
				mu.Lock()
				pass[key]++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	for key, n := range pass {
		if n != 1 {
			t.Fatalf("key %s 在 max=2 下被放行 %d 次，应恰好 1 次", key, n)
		}
	}
	if len(pass) != 50 {
		t.Fatalf("50 个不同 key 应全部放行一次，实际 %d", len(pass))
	}
}

// TestLimiterKeyCapBoundsMemory 验证 key 数量有硬上界。
//
// 攻击者用随机源 IP 刷时，map 会一直涨到 GC 周期才被清理；
// 高速率下「速率 × 窗口时长」可以是无界的 —— OOM 是不可 recover 的。
func TestLimiterKeyCapBoundsMemory(t *testing.T) {
	const keyCap = 50
	l := limiter.NewWithCap(3, time.Minute, keyCap)

	for i := 0; i < 600; i++ {
		key := fmt.Sprintf("10.%d.%d.%d", i/65536, (i/256)%256, i%256)
		l.Fail(key)
		l.Allow(key)
		l.AllowAndConsume(key)
	}

	keys, evicted := l.Stats()
	if keys > keyCap {
		t.Fatalf("key 数应不超过上界 %d，实际 %d", keyCap, keys)
	}
	if evicted == 0 {
		t.Fatal("超出上界的部分应计入 evicted，供监控观察")
	}
	// 被上界挡掉的 key 一律放行 —— 这是有意的取舍：
	// 宁可放过攻击，也不能 OOM（不可用比被限流更糟）。
	if !l.Allow("never-seen-key") {
		t.Fatal("上界已满时应放行新 key（fail-open），不能把正常用户挡在门外")
	}
}

// TestLimiterAllowAndConsumeEnforcesThreshold 确认阈值本身仍然生效。
func TestLimiterAllowAndConsumeEnforcesThreshold(t *testing.T) {
	l := limiter.New(3, time.Minute)
	for i := 1; i <= 3; i++ {
		if !l.AllowAndConsume("a") {
			t.Fatalf("第 %d 次应放行", i)
		}
	}
	for i := 0; i < 5; i++ {
		if l.AllowAndConsume("a") {
			t.Fatalf("达到阈值后第 %d 次仍被放行，阈值失效", i+1)
		}
	}
}

// TestLimiterAllowAndConsumeDisabled 关闭状态下恒定放行。
func TestLimiterAllowAndConsumeDisabled(t *testing.T) {
	l := limiter.New(0, time.Minute)
	for i := 0; i < 50; i++ {
		if !l.AllowAndConsume("a") {
			t.Fatal("关闭状态下应始终放行")
		}
	}
}

// TestLimiterNilReceiverSafe nil 接收者不能 panic（Service 未初始化时的防御）。
func TestLimiterNilReceiverSafe(t *testing.T) {
	var l *limiter.FailLimiter
	if !l.AllowAndConsume("a") {
		t.Fatal("nil 限流器应放行")
	}
	l.Fail("a")
	l.Reset("a")
	keys, evicted := l.Stats()
	if keys != 0 || evicted != 0 {
		t.Fatal("nil 限流器的 Stats 应返回零值")
	}
}

// ---------------------------------------------------------------------------
// 2. JWT：算法白名单 + issuer 校验
// ---------------------------------------------------------------------------

// signClaims 用指定密钥与算法签一个令牌。
func signClaims(t *testing.T, method upstream.SigningMethod, secret string, c jwt.Claims) string {
	t.Helper()
	tok, err := upstream.NewWithClaims(method, c).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("构造令牌失败: %v", err)
	}
	return tok
}

// baseClaims 造一个字段完整、只差算法/issuer 的载荷。
func baseClaims(issuer string) jwt.Claims {
	now := time.Now()
	return jwt.Claims{
		UserID:   1,
		Username: "admin",
		Type:     jwt.AccessToken,
		Epoch:    0,
		RegisteredClaims: upstream.RegisteredClaims{
			Issuer:    issuer,
			Subject:   "admin",
			IssuedAt:  upstream.NewNumericDate(now),
			NotBefore: upstream.NewNumericDate(now),
			ExpiresAt: upstream.NewNumericDate(now.Add(time.Hour)),
		},
	}
}

func newTestJWT(secret, issuer string) *jwt.Manager {
	return jwt.New(config.JWTConfig{
		Secret:        secret,
		Issuer:        issuer,
		AccessExpire:  3600,
		RefreshExpire: 604800,
	})
}

// TestJWTRejectsForeignAlgorithm 验证非 HS256 算法一律拒绝。
//
// 旧代码只断言「是 SigningMethodHMAC 的子类」，HS384 / HS512 同样属于 HMAC，
// 于是攻击者只要知道密钥就能用这些算法签一个能被接受的令牌。
func TestJWTRejectsForeignAlgorithm(t *testing.T) {
	const secret = "jwt-algo-secret"
	m := newTestJWT(secret, "nova-test")

	for _, tc := range []struct {
		alg    string
		method upstream.SigningMethod
	}{
		{"HS384", upstream.SigningMethodHS384},
		{"HS512", upstream.SigningMethodHS512},
	} {
		t.Run(tc.alg, func(t *testing.T) {
			tok := signClaims(t, tc.method, secret, baseClaims("nova-test"))
			if _, err := m.Parse(tok); err == nil {
				t.Fatalf("算法 %s 应被拒绝（只允许 HS256）", tc.alg)
			}
		})
	}

	// HS256 且 issuer 正确时必须放行 —— 确认白名单没有误伤
	good := signClaims(t, upstream.SigningMethodHS256, secret, baseClaims("nova-test"))
	if _, err := m.Parse(good); err != nil {
		t.Fatalf("HS256 + 正确 issuer 的令牌应放行，实际: %v", err)
	}
}

// TestJWTRejectsForeignIssuer 验证 issuer 不匹配的令牌被拒绝。
//
// 多环境共用一份 secret 是很常见的配置。不校验 issuer 时，
// 别的系统签发的令牌能直接拿到本服务的会话用。
func TestJWTRejectsForeignIssuer(t *testing.T) {
	const secret = "jwt-issuer-secret"
	m := newTestJWT(secret, "nova-production")

	other := signClaims(t, upstream.SigningMethodHS256, secret, baseClaims("some-other-system"))
	if _, err := m.Parse(other); err == nil {
		t.Fatal("issuer 不匹配的令牌应被拒绝")
	}

	empty := signClaims(t, upstream.SigningMethodHS256, secret, baseClaims(""))
	if _, err := m.Parse(empty); err == nil {
		t.Fatal("issuer 为空的令牌应被拒绝")
	}

	same := signClaims(t, upstream.SigningMethodHS256, secret, baseClaims("nova-production"))
	if _, err := m.Parse(same); err != nil {
		t.Fatalf("同 issuer 的令牌应放行，实际: %v", err)
	}
}

// TestJWTRejectsWrongSecret 确认密钥不对时拒绝（回归锚点，防误改）。
func TestJWTRejectsWrongSecret(t *testing.T) {
	m := newTestJWT("right-secret", "nova-test")
	tok := signClaims(t, upstream.SigningMethodHS256, "wrong-secret", baseClaims("nova-test"))
	if _, err := m.Parse(tok); err == nil {
		t.Fatal("密钥不匹配的令牌应被拒绝")
	}
}

// TestJWTRejectsAlgNone 验证 alg=none 攻击被挡住。
func TestJWTRejectsAlgNone(t *testing.T) {
	m := newTestJWT("jwt-none-secret", "nova-test")
	tok, err := upstream.NewWithClaims(upstream.SigningMethodNone, baseClaims("nova-test")).
		SignedString(upstream.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Skipf("当前库版本不支持构造 alg=none 令牌: %v", err)
	}
	if _, err := m.Parse(tok); err == nil {
		t.Fatal("alg=none 令牌必须被拒绝")
	}
}

// TestJWTRejectsZeroUserID 验证 uid<=0 的令牌被拒绝。
//
// 载荷被截断或伪造时 UserID 会是 0；各处拿它做数据范围判定，
// 0 值会落到「查不到用户」的分支而不是明确报错。
func TestJWTRejectsZeroUserID(t *testing.T) {
	const secret = "jwt-uid-secret"
	m := newTestJWT(secret, "nova-test")

	claims := baseClaims("nova-test")
	claims.UserID = 0
	if _, err := m.Parse(signClaims(t, upstream.SigningMethodHS256, secret, claims)); err == nil {
		t.Fatal("uid=0 的令牌应被拒绝")
	}

	claims.UserID = -1
	if _, err := m.Parse(signClaims(t, upstream.SigningMethodHS256, secret, claims)); err == nil {
		t.Fatal("uid 为负的令牌应被拒绝")
	}
}

// TestJWTExpiredStillReportsExpired 确认过期与无效仍是两种错误。
//
// 请求层靠这个区分决定是「静默续期」还是「直接跳登录」。
func TestJWTExpiredStillReportsExpired(t *testing.T) {
	const secret = "jwt-exp-secret"
	m := newTestJWT(secret, "nova-test")

	past := time.Now().Add(-2 * time.Hour)
	claims := baseClaims("nova-test")
	claims.IssuedAt = upstream.NewNumericDate(past)
	claims.NotBefore = upstream.NewNumericDate(past)
	claims.ExpiresAt = upstream.NewNumericDate(past.Add(time.Minute))

	_, err := m.Parse(signClaims(t, upstream.SigningMethodHS256, secret, claims))
	if err == nil {
		t.Fatal("过期令牌应解析失败")
	}
	if err != jwt.ErrTokenExpired {
		t.Fatalf("过期应返回 ErrTokenExpired，实际: %v", err)
	}

	// 有效期内但 issuer 不符 → 仍是 invalid，不是 expired
	bad := signClaims(t, upstream.SigningMethodHS256, secret, baseClaims("other"))
	if _, err := m.Parse(bad); err != jwt.ErrTokenInvalid {
		t.Fatalf("issuer 不符应返回 ErrTokenInvalid，实际: %v", err)
	}
}

// TestJWTRealLoginTokensStillValid 确认正常登录签发的令牌不受影响。
//
// 上面几条都是手工构造的畸形令牌，这条是回归锚点：
// 确保新加的校验没有把正常流程一起拒掉。
func TestJWTRealLoginTokensStillValid(t *testing.T) {
	access, refresh := loginPair(t, adminAccount, adminSecret)
	if access == "" || refresh == "" {
		t.Fatal("登录应返回双令牌")
	}
	if res := call(t, http.MethodGet, "/api/v1/profile", access, nil); res.Code != 0 {
		t.Fatalf("新令牌应可用于访问接口，实际 code=%d msg=%s", res.Code, res.Msg)
	}
	// 刷新链路也要通（走 ParseRefresh，会额外校验 Type）
	if res := call(t, http.MethodPost, "/api/v1/auth/refresh", "",
		map[string]any{"refreshToken": refresh}); res.Code != 0 {
		t.Fatalf("刷新应成功，实际 code=%d msg=%s", res.Code, res.Msg)
	}
}

// ---------------------------------------------------------------------------
// 3. 分页 OFFSET 上界
// ---------------------------------------------------------------------------

// TestPageOffsetUpperBound 验证页码不能把 OFFSET 撑到天上。
//
// PageSize 封顶了但页码没有：`?page=99999999` 会算出约 2×10⁹ 的 OFFSET，
// PostgreSQL 必须先扫描丢弃这 2 亿行才返回 LIMIT 条 —— 单请求即可占满连接。
func TestPageOffsetUpperBound(t *testing.T) {
	cases := []struct {
		name     string
		q        page.Query
		wantMax  int
		wantPage int
	}{
		{"超大页码", page.Query{Page: 99_999_999, PageSize: 200}, 1_000_000, 5001},
		{"极大页码", page.Query{Page: 1 << 40, PageSize: 200}, 1_000_000, 5001},
		{"接近整型上限", page.Query{Page: 1 << 62, PageSize: 200}, 1_000_000, 5001},
		{"负数页码", page.Query{Page: -5, PageSize: 10}, 0, 1},
		{"零页码", page.Query{Page: 0, PageSize: 10}, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.q
			q.Normalize()
			got := q.Offset()
			if got > tc.wantMax {
				t.Fatalf("OFFSET=%d 超过上界 %d", got, tc.wantMax)
			}
			if got < 0 {
				t.Fatalf("OFFSET=%d 不能为负（整型溢出）", got)
			}
			if q.Page != tc.wantPage {
				t.Fatalf("页码应收敛到 %d，实际 %d", tc.wantPage, q.Page)
			}
		})
	}
}

// TestPageNormalizeKeepsReasonableValues 确认正常分页参数不被收敛逻辑影响。
func TestPageNormalizeKeepsReasonableValues(t *testing.T) {
	q := page.Query{Page: 3, PageSize: 20, Order: "asc"}
	q.Normalize()
	if q.Offset() != 40 {
		t.Fatalf("第 3 页每页 20 条的 OFFSET 应为 40，实际 %d", q.Offset())
	}
	if q.Limit() != 20 {
		t.Fatalf("每页条数应保持 20，实际 %d", q.Limit())
	}

	// PageSize 仍按 maxPageSize 收敛
	big := page.Query{Page: 1, PageSize: 9999}
	big.Normalize()
	if big.PageSize != 200 {
		t.Fatalf("PageSize 应收敛到 200，实际 %d", big.PageSize)
	}
}

// TestPageHugeNumberThroughHTTP 通过真实 HTTP 请求验证。
func TestPageHugeNumberThroughHTTP(t *testing.T) {
	access, _ := loginPair(t, adminAccount, adminSecret)
	for _, p := range []string{
		"/api/v1/system/users?page=99999999999&pageSize=200",
		"/api/v1/system/users?page=-1&pageSize=200",
		"/api/v1/system/roles?page=9223372036854775807&pageSize=200",
		"/api/v1/system/depts?page=999999999&pageSize=99999",
	} {
		res := call(t, http.MethodGet, p, access, nil)
		if res.Code != 0 {
			t.Fatalf("%s 应正常返回（空列表）而非报错，实际 code=%d msg=%s", p, res.Code, res.Msg)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. 角色聚合查询的错误传播
// ---------------------------------------------------------------------------

// TestRoleListSurfacesDBError 验证聚合查询出错时接口报错而非返回空数据。
//
// 旧代码 `_ = s.db...Scan(...)` 吞掉 error，DB 抖动时角色列表会显示
// 「所有角色都没有菜单、没有用户」—— 把故障伪装成业务结论，
// 管理员据此操作会做出错误判断。
func TestRoleListSurfacesDBError(t *testing.T) {
	access, _ := loginPair(t, adminAccount, adminSecret)

	// 先确认正常路径可用
	ok := call(t, http.MethodGet, "/api/v1/system/roles?page=1&pageSize=20", access, nil)
	if ok.Code != 0 {
		t.Fatalf("角色列表应可查询，实际 code=%d msg=%s", ok.Code, ok.Msg)
	}
	if len(ok.List) == 0 {
		t.Fatal("种子数据里应有角色")
	}

	// 让聚合查询必然失败：把关联表改名
	if err := testDB.Exec("ALTER TABLE sys_role_menus RENAME TO sys_role_menus_bak_t4").Error; err != nil {
		t.Skipf("无法重命名表（权限不足），跳过: %v", err)
	}
	restored := false
	defer func() {
		if !restored {
			testDB.Exec("ALTER TABLE sys_role_menus_bak_t4 RENAME TO sys_role_menus")
		}
	}()

	res := call(t, http.MethodGet, "/api/v1/system/roles?page=1&pageSize=20", access, nil)
	if res.Code == 0 {
		t.Fatalf("聚合查询失败时必须报错，不能返回空菜单列表（会把故障伪装成「没有菜单」）")
	}
	if res.Code != 50200 {
		t.Logf("返回码为 %d（非数据库错误 50200），仍属「有报错」，符合预期", res.Code)
	}

	// 恢复表名后再确认正常路径没被这次用例搞坏
	if err := testDB.Exec("ALTER TABLE sys_role_menus_bak_t4 RENAME TO sys_role_menus").Error; err != nil {
		t.Fatalf("恢复表名失败: %v", err)
	}
	restored = true

	again := call(t, http.MethodGet, "/api/v1/system/roles?page=1&pageSize=20", access, nil)
	if again.Code != 0 {
		t.Fatalf("恢复后角色列表应可查询，实际 code=%d msg=%s", again.Code, again.Msg)
	}
}

// TestRoleDetailSurfacesDBError 验证详情页同样不吞错。
func TestRoleDetailSurfacesDBError(t *testing.T) {
	access, _ := loginPair(t, adminAccount, adminSecret)

	list := call(t, http.MethodGet, "/api/v1/system/roles?page=1&pageSize=1", access, nil)
	if list.Code != 0 || len(list.List) == 0 {
		t.Fatalf("取角色列表失败: code=%d", list.Code)
	}
	first, _ := list.List[0].(map[string]any)
	id := num(first, "id")
	if id <= 0 {
		t.Fatalf("角色 ID 无效: %v", first["id"])
	}

	if err := testDB.Exec("ALTER TABLE sys_user_roles RENAME TO sys_user_roles_bak_t4").Error; err != nil {
		t.Skipf("无法重命名表（权限不足），跳过: %v", err)
	}
	defer func() {
		testDB.Exec("ALTER TABLE sys_user_roles_bak_t4 RENAME TO sys_user_roles")
	}()

	res := call(t, http.MethodGet, fmt.Sprintf("/api/v1/system/roles/%d", id), access, nil)
	if res.Code == 0 {
		t.Fatal("详情页的聚合查询失败时必须报错，不能把 userCount 静默置 0")
	}
}
