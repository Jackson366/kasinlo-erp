package user

import "testing"

// buildCells 按列下标拼一行单元格。
func buildCells(vals map[int]string) []string {
	out := make([]string, importColumnCount)
	for i, v := range vals {
		out[i] = v
	}
	return out
}

// TestValidateImportLineSeenOnlyForValidRows 「文件内用户名重复」只应由真正会入库的行触发。
//
// 旧实现只要**用户名本身**合法就立刻记入 seen，哪怕整行因昵称/部门/角色校验失败。
// 于是后面同名且完全合法的行会被误判「与文件中的其他行用户名重复」，
// 而前一行根本不会入库 —— 用户看到两条都失败，且原因指向错误。
func TestValidateImportLineSeenOnlyForValidRows(t *testing.T) {
	svc := &Service{}
	ref := &importRefs{
		depts:     map[string]int64{},
		roles:     map[string]int64{},
		usernames: map[string]bool{},
	}
	seen := map[string]bool{}

	// 第 1 行：用户名合法但昵称为空 —— 必然被拒绝、不会入库
	invalid := buildCells(map[int]string{colUsername: "dupe_user", colNickname: ""})
	if _, errs := svc.validateImportLine(invalid, ref, seen, nil); len(errs) == 0 {
		t.Fatal("昵称为空的行应校验失败")
	}

	// 第 2 行：同名、完全合法 —— 必须通过（前一行没入库，不构成重复）
	valid := buildCells(map[int]string{colUsername: "dupe_user", colNickname: "正常昵称"})
	if _, errs := svc.validateImportLine(valid, ref, seen, nil); len(errs) != 0 {
		t.Fatalf("前一行未入库时，同名合法行不应被判为重复：%v", errs)
	}

	// 第 3 行：与第 2 行同名且同样合法 —— 这时才应判「文件内重复」
	if _, errs := svc.validateImportLine(valid, ref, seen, nil); len(errs) == 0 {
		t.Fatal("两行同名且都合法时，应判为文件内用户名重复")
	}
}
