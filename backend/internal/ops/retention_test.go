package ops

import (
	"testing"
	"time"
)

func mkBackup(day string) BackupFile {
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		panic(err)
	}
	return BackupFile{Path: "/b/" + day + ".dump", Time: t, Size: 1024}
}

func TestPlanRetentionKeepsRecent(t *testing.T) {
	now := mustDate(t, "2026-10-18")
	files := []BackupFile{
		mkBackup("2026-10-18"), mkBackup("2026-10-17"), mkBackup("2026-10-16"),
		mkBackup("2026-10-15"), mkBackup("2026-10-14"),
	}
	// keepCount=3, keepMonths=0：只留最近 3 份 + 当月最新（已在最近 3 份里）
	victims := PlanRetention(files, now, 3, 0)
	if len(victims) != 2 {
		t.Fatalf("应删除 2 份，实际 %d: %+v", len(victims), victims)
	}
	for _, v := range victims {
		if v.Path == "/b/2026-10-18.dump" {
			t.Fatal("最新一份被删了")
		}
	}
}

func TestPlanRetentionNeverDeletesNewest(t *testing.T) {
	files := []BackupFile{mkBackup("2026-10-18")}
	// 参数写错（keepCount=0 / 负数）也不能把唯一备份删掉
	for _, keep := range []int{0, -5} {
		if victims := PlanRetention(files, mustDate(t, "2026-10-18"), keep, 0); len(victims) != 0 {
			t.Fatalf("keepCount=%d 时删除了唯一备份: %+v", keep, victims)
		}
	}
}

func TestPlanRetentionKeepsMonthlyAnchor(t *testing.T) {
	now := mustDate(t, "2026-10-18")
	// 7 月的两份、8 月的一份、9 月的一份，都超出最近 2 份
	files := []BackupFile{
		mkBackup("2026-10-18"), mkBackup("2026-10-17"),
		mkBackup("2026-09-30"), mkBackup("2026-08-15"),
		mkBackup("2026-07-20"), mkBackup("2026-07-01"),
	}
	victims := PlanRetention(files, now, 2, 2) // 覆盖 10 月与 9 月
	kept := map[string]bool{}
	for _, f := range files {
		kept[f.Path] = true
	}
	for _, v := range victims {
		delete(kept, v.Path)
	}
	// 保留：10-18、10-17（最近 2 份）+ 09-30（9 月最新）+ 10 月已有
	// 删除：08-15、07-20、07-01
	if !kept["/b/2026-09-30.dump"] {
		t.Fatal("9 月的月档被删了")
	}
	if len(victims) != 3 {
		t.Fatalf("应删除 3 份，实际 %d: %+v", len(victims), victims)
	}
}

func TestPlanRetentionSameTimeIsStable(t *testing.T) {
	now := mustDate(t, "2026-10-18")
	t1 := mustDate(t, "2026-10-10")
	files := []BackupFile{
		{Path: "/b/a.dump", Time: t1}, {Path: "/b/b.dump", Time: t1}, {Path: "/b/c.dump", Time: t1},
	}
	first := PlanRetention(files, now, 1, 0)
	for i := 0; i < 5; i++ {
		again := PlanRetention(files, now, 1, 0)
		if len(again) != len(first) || again[0].Path != first[0].Path {
			t.Fatalf("同一批文件的结果不稳定: %+v vs %+v", first, again)
		}
	}
	// 时间相同按路径倒序，c 最大 → 保留 c，删 a、b
	if first[0].Path != "/b/b.dump" || first[1].Path != "/b/a.dump" {
		t.Fatalf("相同时间的选择不符合预期: %+v", first)
	}
}

func mustDate(t *testing.T, day string) time.Time {
	t.Helper()
	v, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
