package tree

import "testing"

// node 是测试用节点，只保留「我是谁」与「我的孩子」，便于断言结构。
type node struct {
	ID       int64
	Children []*node
}

type item struct {
	id     int64
	parent int64
}

func newNode(i item) *node { return &node{ID: i.id} }

func keys(i item) (int64, int64) { return i.id, i.parent }

func attach(parent, child *node) { parent.Children = append(parent.Children, child) }

// build 是各用例共用的组装入口。
func build(items []item) []*node {
	return Build(items, newNode, keys, attach)
}

// flatten 展平森林，用于验证「节点一个都没丢」。
func flatten(roots []*node) []*node {
	var out []*node
	var walk func(*node)
	walk = func(n *node) {
		out = append(out, n)
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	return out
}

func ids(nodes []*node) []int64 {
	out := make([]int64, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.ID)
	}
	return out
}

func sameIDs(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestBuildNestsChildren(t *testing.T) {
	roots := build([]item{
		{1, 0}, {2, 1}, {3, 1}, {4, 2},
	})

	if got := ids(roots); !sameIDs(got, []int64{1}) {
		t.Fatalf("根节点应只有 1，实际 %v", got)
	}
	if got := ids(roots[0].Children); !sameIDs(got, []int64{2, 3}) {
		t.Fatalf("节点 1 的子节点顺序应与输入一致（2,3），实际 %v", got)
	}
	if got := ids(roots[0].Children[0].Children); !sameIDs(got, []int64{4}) {
		t.Fatalf("节点 2 的子节点应为 4，实际 %v", got)
	}
}

// TestBuildKeepsOrphansAsRoots 覆盖「父节点被裁掉 / 父 ID 非法」这一最容易丢数据的边界：
// 挂不上的节点必须按根保留，而不是被静默丢弃。
func TestBuildKeepsOrphansAsRoots(t *testing.T) {
	roots := build([]item{
		{1, 0},
		{2, 1},
		{9, 99}, // 父 99 不存在
	})

	if got := ids(roots); !sameIDs(got, []int64{1, 9}) {
		t.Fatalf("父节点不存在的节点应按根保留，实际根节点 %v", got)
	}
	if len(flatten(roots)) != 3 {
		t.Fatalf("节点不得丢失，期望 3 个，实际 %d 个", len(flatten(roots)))
	}
}

// TestBuildTreatsParentZeroAsRoot 显式锁定「parent_id = 0 即根」的语义。
// 若哪天有人把 `parentID != 0` 的判断删掉，而恰好又没有 id 为 0 的节点，
// 这条用例仍然会红，因为它断言的是根集合本身。
func TestBuildTreatsParentZeroAsRoot(t *testing.T) {
	roots := build([]item{{10, 0}, {11, 0}})
	if got := ids(roots); !sameIDs(got, []int64{10, 11}) {
		t.Fatalf("parent_id=0 的节点都应视为根，实际 %v", got)
	}
	for _, r := range roots {
		if len(r.Children) != 0 {
			t.Fatalf("根节点不应有子节点，节点 %d 有 %d 个", r.ID, len(r.Children))
		}
	}
}

// TestBuildNoDataLostOnDuplicateIDs 锁定「重复 ID 后者覆盖前者」的既有语义：
// 索引被后一个节点覆盖，但每个输入项仍各自保留在结果里。
func TestBuildNoDataLostOnDuplicateIDs(t *testing.T) {
	roots := build([]item{
		{1, 0},
		{1, 0}, // 重复 ID
		{2, 1},
	})

	if len(flatten(roots)) != 3 {
		t.Fatalf("每个输入项都应对应一个节点，期望 3 个，实际 %d 个", len(flatten(roots)))
	}
	// 索引被第二个 1 覆盖，因此子节点 2 挂在第二个 1 上
	if len(roots) != 2 {
		t.Fatalf("两个 id=1 的项都应是根，实际根节点数 %d", len(roots))
	}
	if got := ids(roots[1].Children); !sameIDs(got, []int64{2}) {
		t.Fatalf("子节点应挂到后者（索引被覆盖的那个），实际 %v", got)
	}
	if len(roots[0].Children) != 0 {
		t.Fatalf("先出现的同 ID 节点不应有子节点")
	}
}

func TestBuildEmptyInput(t *testing.T) {
	roots := build(nil)
	if len(roots) != 0 {
		t.Fatalf("空输入应返回空森林，实际 %d 个根", len(roots))
	}
}

// TestBuildDoesNotMutateOrderOfRoots 确认根节点顺序与输入顺序一致，
// 前端菜单/部门树依赖这个顺序保持稳定。
func TestBuildDoesNotMutateOrderOfRoots(t *testing.T) {
	roots := build([]item{
		{5, 99},
		{1, 0},
		{7, 98},
	})
	if got := ids(roots); !sameIDs(got, []int64{5, 1, 7}) {
		t.Fatalf("根节点顺序应与输入一致，实际 %v", got)
	}
}
