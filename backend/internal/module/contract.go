// Package module 定义「模块自描述清单」与登记表。
//
// 背景：原先「加一个菜单」要在 5 个地方各改一次 —— 模块的 route.go 写权限码、
// seed.go 的 menuDefs() 手工分配菜单 ID、seed.go 的 builtinRoleGrants() 再把
// ID 抄一遍授权、前端加页面、最后还要去菜单管理页手工录一条。其中权限码写两遍
// 最危险：漂移之后的症状是「菜单点得进去、接口 403」，排查成本极高。
//
// 现在每个模块（系统模块与业务模块一视同仁）用一份 Module 描述自己：
// 权限命名空间、路由注册函数、菜单树。seed 从登记表聚合出内置菜单，
// 「加一个菜单」收敛成写 1 份清单。
//
// 本包刻意不 import 任何具体模块 —— 把模块塞进登记表由组装层（cmd/server）做。
// 这条约束保证框架层对业务零依赖：业务模块可以整块删掉，框架照常编译启动。
package module

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// 菜单类型，取值与 model 包中的同名常量一致。
const (
	TypeDir    = "dir"    // 目录：只做侧边栏分组，不注册路由
	TypeMenu   = "menu"   // 菜单：对应一个页面
	TypeButton = "button" // 按钮：只是一个权限点，不出现在侧边栏
)

// MenuNode 是一个菜单节点。
//
// 字段刻意留空即用默认值，让清单读起来接近自然语言：
// 只写 Name/Title/Path/Component/Permission 就能表达一个页面，
// Type 留空按菜单处理，Sort 留空按声明顺序排。
type MenuNode struct {
	Name       string // 路由名，全局唯一，如 SystemUser
	Title      string // 显示名称，如「用户管理」
	Icon       string // 图标名，如 User
	Path       string // 路由路径，如 /system/user
	Component  string // 页面组件路径（相对 src/views），如 system/user/index
	Redirect   string // 目录重定向目标
	Permission string // 权限标识，如 system:user:create
	Type       string // TypeDir / TypeMenu / TypeButton，留空按 TypeMenu
	Sort       int    // 同级排序，留空按声明顺序
	Hidden     bool   // 是否在侧边栏隐藏
	KeepAlive  bool   // 页面是否缓存
	IsExternal bool   // 是否外链
	Children   []MenuNode

	// namespace 由所属模块的 Namespace 盖章，不出现在清单里。
	//
	// 为什么不直接在节点上写：模块已经声明过自己的命名空间了，每个权限码
	// 再重复一遍前缀纯属噪音。盖章在 Build/Group 归一化时完成，
	// 这样校验仍然能逐节点判断「权限码有没有用错命名空间」。
	namespace string
}

// TypeOf 返回菜单类型，留空按「菜单」处理。
func (n MenuNode) TypeOf() string {
	if strings.TrimSpace(n.Type) == "" {
		return TypeMenu
	}
	return n.Type
}

// Module 是一个模块的自描述清单。
type Module struct {
	// Namespace 是该模块的权限命名空间，如 system / inv / crm。
	// 清单里所有权限码都必须以它开头，由 Build 校验。
	Namespace string

	// Register 注册本模块路由。需要配置的模块在自己的包内构造闭包
	//（如 log.Module(cfg.Security)），于是本包不必依赖 config 包。
	// 纯菜单模块（只有权限点、没有接口）可以留 nil。
	Register func(*gin.RouterGroup)

	// Menus 是本模块声明的菜单树，顶层节点会挂在根目录下。
	Menus []MenuNode
}

// registry 保存全部已登记模块。启动时一次性写入，之后只读。
var registry []Module

// Build 登记全部模块并校验清单，由组装层（cmd/server）调用。
//
// 这里是全项目唯一知道「一共有哪些模块」的地方。校验放在启动时而不是运行期：
// 权限码写错、路由名撞车这类问题，运行期的表现是「菜单能进但接口 403」，
// 让人在权限系统里绕半天；启动即失败则一眼看到错在哪。
func Build(mods ...Module) error {
	normalized := make([]Module, 0, len(mods))
	for _, m := range mods {
		normalized = append(normalized, m.normalized())
	}
	if err := validate(normalized); err != nil {
		return err
	}
	registry = normalized
	return nil
}

// Group 把一个目录节点与若干模块合成一个模块。
//
// 用途：像「系统管理」这种把多个模块收在一个目录下的分组，属于
// 「哪些模块放在一起展示」的组装决策 —— 是组装层的职责，不是 user 或 role
// 模块自己的事。让模块声明自己挂在哪个目录下，等于把导航结构泄漏进模块内部。
//
// 分组同时把子模块的路由注册串起来：否则被 Group 包住的模块路由会整块丢失，
// 而这种丢失不会有任何编译错误，只表现为接口 404。
//
// 用法（组装层）：
//
//	module.Group(module.MenuNode{Name: "System", Title: "系统管理", Path: "/system", Sort: 2},
//		user.Module(), role.Module(), ...)
func Group(dir MenuNode, mods ...Module) Module {
	dir.Type = TypeDir
	dir.Children = nil

	registers := make([]func(*gin.RouterGroup), 0, len(mods))
	ns := ""
	for _, m := range mods {
		if ns == "" {
			ns = m.Namespace
		}
		if m.Register != nil {
			registers = append(registers, m.Register)
		}
		dir.Children = append(dir.Children, stamp(m.Namespace, m.Menus)...)
	}

	return Module{
		// 目录本身没有权限码，Namespace 只作为「本组引用哪个命名空间」的提示；
		// 组内每个节点已经各自盖好了自己模块的命名空间。
		Namespace: ns,
		Register: func(rg *gin.RouterGroup) {
			for _, register := range registers {
				register(rg)
			}
		},
		Menus: []MenuNode{dir},
	}
}

// normalized 返回把各节点盖好命名空间的副本。
func (m Module) normalized() Module {
	m.Menus = stamp(m.Namespace, m.Menus)
	return m
}

// stamp 递归给还没盖过章的节点写上命名空间。
//
// 「只在为空时写」是有意的：Group 在构造时已经给子模块的节点盖过章了，
// Build 再做一次归一化时不能把它们覆盖成组自己的命名空间。
func stamp(ns string, nodes []MenuNode) []MenuNode {
	if len(nodes) == 0 {
		return nodes
	}
	out := make([]MenuNode, len(nodes))
	for i, n := range nodes {
		if n.namespace == "" {
			n.namespace = ns
		}
		n.Children = stamp(ns, n.Children)
		out[i] = n
	}
	return out
}

// All 返回全部已登记模块，顺序与 Build 的入参一致。
func All() []Module { return registry }

// Reset 清空登记表，仅供测试使用。
func Reset() { registry = nil }

// validate 校验全部清单。入参要求已经过 normalized（命名空间已盖章）。
func validate(mods []Module) error {
	if len(mods) == 0 {
		return fmt.Errorf("模块登记表为空：组装层必须至少登记一个模块")
	}

	seen := &collisions{
		names:      map[string]string{},
		perms:      map[string]string{},
		paths:      map[string]string{},
		components: map[string]string{},
	}
	nsSeen := map[string]bool{}

	for _, m := range mods {
		for _, node := range m.Menus {
			if err := validateNode(node, seen, nsSeen); err != nil {
				return err
			}
		}
	}
	return nil
}

// collisions 记录跨模块需要全局唯一的键。撞车时能报出「谁和谁撞了」，
// 而不是干巴巴一句重复 —— 定位成本差一个数量级。
type collisions struct {
	names      map[string]string // 路由名 -> 位置
	perms      map[string]string // 权限码 -> 位置
	paths      map[string]string // 路由路径 -> 位置
	components map[string]string // 组件路径 -> 位置
}

func (c *collisions) claim(bucket map[string]string, key, where, what string) error {
	if key == "" {
		return nil
	}
	if prev, ok := bucket[key]; ok {
		return fmt.Errorf("%s %q 被重复声明：%s 与 %s", what, key, prev, where)
	}
	bucket[key] = where
	return nil
}

func validateNode(n MenuNode, c *collisions, nsSeen map[string]bool) error {
	where := fmt.Sprintf("节点 %q", n.Name)

	if strings.TrimSpace(n.Name) == "" {
		return fmt.Errorf("存在缺少 Name 的菜单节点")
	}
	if strings.TrimSpace(n.Title) == "" {
		return fmt.Errorf("%s 缺少 Title", where)
	}
	if err := c.claim(c.names, n.Name, where, "路由名"); err != nil {
		return err
	}

	switch n.TypeOf() {
	case TypeDir:
		if strings.TrimSpace(n.Path) == "" {
			return fmt.Errorf("%s 是目录，必须声明 Path", where)
		}
	case TypeMenu:
		if strings.TrimSpace(n.Path) == "" {
			return fmt.Errorf("%s 是菜单，必须声明 Path", where)
		}
		if strings.TrimSpace(n.Component) == "" {
			return fmt.Errorf("%s 是菜单，必须声明 Component", where)
		}
	case TypeButton:
		if strings.TrimSpace(n.Permission) == "" {
			return fmt.Errorf("%s 是按钮，必须声明 Permission（按钮存在的意义就是权限点）", where)
		}
		if len(n.Children) > 0 {
			return fmt.Errorf("%s 是按钮，不能有子节点", where)
		}
	default:
		return fmt.Errorf("%s 的类型 %q 非法，只能是 dir / menu / button", where, n.Type)
	}

	if strings.HasPrefix(n.Path, "//") {
		return fmt.Errorf("%s 的路径 %q 不能以 // 开头", where, n.Path)
	}
	if err := c.claim(c.paths, n.Path, where, "路由路径"); err != nil {
		return err
	}

	// 组件路径的写法是 vue-router 那侧的约定（src/views/<Component>.vue）。
	// 写成 /system/user/index 或 system/user/index.vue 在运行期只会 console.warn，
	// 用户看到的是白屏；在启动时挡掉，代价是一次重启。
	if n.Component != "" {
		if strings.HasPrefix(n.Component, "/") {
			return fmt.Errorf("%s 的组件路径 %q 不应以 / 开头（它是相对 src/views 的路径）", where, n.Component)
		}
		if strings.HasSuffix(n.Component, ".vue") {
			return fmt.Errorf("%s 的组件路径 %q 不应带 .vue 后缀", where, n.Component)
		}
		if err := c.claim(c.components, n.Component, where, "组件路径"); err != nil {
			return err
		}
	}

	if n.Permission != "" {
		ns := n.namespace
		if ns == "" {
			return fmt.Errorf("%s 声明了权限码 %q，但所属模块没有声明 Namespace", where, n.Permission)
		}
		if !nsSeen[ns] {
			if strings.Contains(ns, ":") {
				return fmt.Errorf("模块命名空间 %q 不能包含冒号", ns)
			}
			nsSeen[ns] = true
		}
		if !strings.HasPrefix(n.Permission, ns+":") {
			return fmt.Errorf("%s 的权限码 %q 未使用本模块命名空间 %q，应写成 %s:…",
				where, n.Permission, ns, ns)
		}
		if err := c.claim(c.perms, n.Permission, where, "权限码"); err != nil {
			return err
		}
	}

	for _, child := range n.Children {
		if err := validateNode(child, c, nsSeen); err != nil {
			return err
		}
	}
	return nil
}
