package module

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// validModule 返回一份合法的清单，供各用例按需改坏某一处。
func validModule() Module {
	return Module{
		Namespace: "system",
		Menus: []MenuNode{
			{
				Name: "SystemUser", Title: "用户管理", Icon: "User",
				Path: "/system/user", Component: "system/user/index",
				Permission: "system:user:list",
				Children: []MenuNode{
					{Name: "SystemUserCreate", Title: "新增用户", Type: TypeButton, Permission: "system:user:create"},
				},
			},
		},
	}
}

func buildErr(t *testing.T, mods ...Module) error {
	t.Helper()
	t.Cleanup(Reset)
	return Build(mods...)
}

// TestBuildAcceptsValidManifest 合法清单应通过并可按契约类型取回。
func TestBuildAcceptsValidManifest(t *testing.T) {
	if err := buildErr(t, validModule()); err != nil {
		t.Fatalf("合法清单不应报错: %v", err)
	}
	if got := len(All()); got != 1 {
		t.Fatalf("登记表应有 1 个模块，实际 %d", got)
	}
}

// TestBuildRejectsBadManifests 校验必须挡住的几类错误。
//
// 这些错误在运行期的表现都很难查：权限码写错 →「菜单点得进去、接口 403」；
// 组件路径写错 → 白屏 + 一条容易被忽略的 console.warn；
// 路由名撞车 → 动态路由互相覆盖。放到启动时失败，代价只是一次重启。
func TestBuildRejectsBadManifests(t *testing.T) {
	cases := []struct {
		name string
		mods []Module
		want string
	}{
		{
			name: "空登记表",
			mods: nil,
			want: "登记表为空",
		},
		{
			name: "权限码用了别人的命名空间",
			mods: []Module{func() Module {
				m := validModule()
				m.Menus[0].Permission = "inv:user:list"
				return m
			}()},
			want: "未使用本模块命名空间",
		},
		{
			name: "跨模块路由名重复",
			mods: []Module{
				validModule(),
				func() Module {
					m := validModule()
					m.Namespace = "inv"
					m.Menus[0].Permission = "inv:order:list"
					m.Menus[0].Path = "/inv/order"
					m.Menus[0].Component = "inv/order/index"
					m.Menus[0].Children = nil // 去掉按钮，避免权限码跟着撞车
					return m
				}(),
			},
			want: "被重复声明",
		},
		{
			name: "权限码重复",
			mods: []Module{func() Module {
				m := validModule()
				m.Menus[0].Children = append(m.Menus[0].Children, MenuNode{
					Name: "SystemUserCreate2", Title: "重复权限", Type: TypeButton,
					Permission: "system:user:create",
				})
				return m
			}()},
			want: "被重复声明",
		},
		{
			name: "组件路径带 .vue 后缀",
			mods: []Module{func() Module {
				m := validModule()
				m.Menus[0].Component = "system/user/index.vue"
				return m
			}()},
			want: "不应带 .vue 后缀",
		},
		{
			name: "组件路径以 / 开头",
			mods: []Module{func() Module {
				m := validModule()
				m.Menus[0].Component = "/system/user/index"
				return m
			}()},
			want: "不应以 / 开头",
		},
		{
			name: "按钮没有权限码",
			mods: []Module{func() Module {
				m := validModule()
				m.Menus[0].Children[0].Permission = ""
				return m
			}()},
			want: "是按钮，必须声明 Permission",
		},
		{
			name: "菜单缺少组件路径",
			mods: []Module{func() Module {
				m := validModule()
				m.Menus[0].Component = ""
				return m
			}()},
			want: "必须声明 Component",
		},
		{
			name: "菜单类型非法",
			mods: []Module{func() Module {
				m := validModule()
				m.Menus[0].Type = "page"
				return m
			}()},
			want: "类型",
		},
		{
			name: "声明了权限码却没有命名空间",
			mods: []Module{func() Module {
				m := validModule()
				m.Namespace = ""
				return m
			}()},
			want: "没有声明 Namespace",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := buildErr(t, tc.mods...)
			if err == nil {
				t.Fatalf("应当报错（期望包含 %q），但 Build 通过了", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("错误信息应包含 %q，实际: %v", tc.want, err)
			}
		})
	}
}

// TestGroupComposesRoutesAndMenus 分组必须同时把子模块的菜单与路由带上。
//
// 路由这一半尤其重要：漏掉它不会有任何编译错误，只表现为接口整块 404 ——
// 「菜单能看见、点进去什么都加载不出来」，排查方向很容易被带到前端去。
func TestGroupComposesRoutesAndMenus(t *testing.T) {
	var called []string

	mod := Group(
		MenuNode{Name: "System", Title: "系统管理", Icon: "Setting", Path: "/system", Sort: 2},
		Module{
			Namespace: "system",
			Register: func(rg *gin.RouterGroup) {
				called = append(called, "user")
				rg.GET("/system/users", func(*gin.Context) {})
			},
			Menus: validModule().Menus,
		},
		Module{
			Namespace: "system",
			Register:  func(*gin.RouterGroup) { called = append(called, "role") },
			Menus: []MenuNode{{
				Name: "SystemRole", Title: "角色管理", Path: "/system/role",
				Component: "system/role/index", Permission: "system:role:list",
			}},
		},
	)

	if len(mod.Menus) != 1 {
		t.Fatalf("分组应产出一个顶层节点，实际 %d 个", len(mod.Menus))
	}
	dir := mod.Menus[0]
	if dir.TypeOf() != TypeDir {
		t.Fatalf("分组顶层应为目录，实际 %q", dir.TypeOf())
	}
	if len(dir.Children) != 2 {
		t.Fatalf("目录下应有 2 个子节点，实际 %d", len(dir.Children))
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if mod.Register == nil {
		t.Fatal("分组必须提供 Register，否则子模块路由会整块丢失")
	}
	mod.Register(engine.Group("/api"))

	if len(called) != 2 {
		t.Fatalf("两个子模块的 Register 都应被调用，实际 %v", called)
	}
	routes := engine.Routes()
	if len(routes) != 1 || routes[0].Path != "/api/system/users" {
		t.Fatalf("子模块路由未注册成功: %+v", routes)
	}
}

// TestGroupStampsChildNamespace 分组的子节点要保留各自模块的命名空间。
//
// 若被子模块与父级混用（例如分组用统一的 system 覆盖所有子模块），
// 一个 inv 业务模块挂进分组后，校验就会误判它的权限码「用错了命名空间」。
func TestGroupStampsChildNamespace(t *testing.T) {
	mod := Group(
		MenuNode{Name: "Root", Title: "根", Path: "/root", Sort: 1},
		Module{Namespace: "system", Menus: []MenuNode{
			{Name: "SystemUser", Title: "用户管理", Path: "/system/user", Component: "system/user/index", Permission: "system:user:list"},
		}},
		Module{Namespace: "inv", Menus: []MenuNode{
			{Name: "InvOrder", Title: "订单管理", Path: "/inv/order", Component: "inv/order/index", Permission: "inv:order:list"},
		}},
	)
	// 走一遍完整装配：混用命名空间时这里会直接报「未使用本模块命名空间」
	t.Cleanup(Reset)
	if err := Build(mod); err != nil {
		t.Fatalf("混合命名空间的分组应通过校验: %v", err)
	}
	children := All()[0].Menus[0].Children
	if children[0].namespace != "system" || children[1].namespace != "inv" {
		t.Fatalf("子节点命名空间未按各自模块盖章: %q / %q",
			children[0].namespace, children[1].namespace)
	}
}
