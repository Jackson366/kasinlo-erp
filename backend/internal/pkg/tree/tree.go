// Package tree 收敛「按 parent_id 把扁平列表组装成树」这一重复逻辑。
//
// dept 的部门树、menu 的菜单树、menu 的前端路由树曾经各写一遍同样的
// 挂载循环：建 id->节点映射，再逐节点挂到父节点的 Children 或 roots。
// 三份拷贝的真正风险是行为漂移 —— 例如某一处忘了 `ParentID != 0` 的判断，
//
//	parent_id=0 的根节点会被当成「挂在 0 号节点下」而整树消失，
//	而这种退化在单元测试里未必红（数据里恰好没有 parent_id=0 的行时）。
//
// 泛型签名说明：Go 没有字段约束，节点构造与挂载方式只能由调用方以函数传入，
// 但「映射 + 挂载」这段易错的循环只有这一份实现。
// keys 作用在模型项上而非节点上：节点是给前端看的 DTO，不一定带 ParentID
// （menu.Route 就不带），而模型项一定带。
package tree

// Build 把扁平列表组装成森林，返回所有根节点（按 items 顺序）。
//
// newNode    由模型项构造节点（字段映射留在调用方，各树字段不同）
// keys       返回模型项的 (自身 ID, 父节点 ID)
// attach     把 child 挂到 parent 上（通常是 append 到 Children）
//
// 与旧手写实现保持同一套边界语义：
//   - 父 ID 在映射中不存在（父节点被裁掉/父 ID 非法）→ 该节点按根处理；
//   - 父 ID 为 0 → 按根处理（0 不是任何节点的 ID，映射里必然不存在，
//     但显式判断能让意图不依赖「0 号节点不存在」这个隐含事实）；
//   - items 中的重复 ID：映射后者覆盖前者，与旧实现一致（主键唯一时不出现）。
func Build[T any, N any](
	items []T,
	newNode func(T) *N,
	keys func(T) (id, parentID int64),
	attach func(parent, child *N),
) []*N {
	type pair struct {
		item T
		node *N
	}
	pairs := make([]pair, 0, len(items))
	index := make(map[int64]*N, len(items))
	for _, it := range items {
		n := newNode(it)
		id, _ := keys(it)
		index[id] = n
		pairs = append(pairs, pair{it, n})
	}
	roots := make([]*N, 0, len(items))
	for _, p := range pairs {
		_, parentID := keys(p.item)
		if parent, ok := index[parentID]; ok && parentID != 0 {
			attach(parent, p.node)
		} else {
			roots = append(roots, p.node)
		}
	}
	return roots
}
