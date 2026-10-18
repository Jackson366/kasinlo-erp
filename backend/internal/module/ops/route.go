package ops

import (
	"github.com/gin-gonic/gin"

	"nova-erp/internal/config"
	"nova-erp/internal/database"
	"nova-erp/internal/middleware"
)

// Register 注册运维模块路由。
// 权限标识：ops:list（查看）/ ops:backup（备份）/ ops:restore（恢复）/
// ops:cleanup（清理）/ ops:upgrade（升级）。
//
// 备份、恢复、清理、升级四类动作各自独立授权：它们的影响面完全不同 ——
// 备份只是读，恢复会建一个库，清理不可逆，升级会替换正在运行的程序。
// 合成一个「ops:admin」看似省事，实际是逼着把所有权限发给同一个人。
func Register(rg *gin.RouterGroup, cfg *config.Config) {
	h := NewHandler(NewService(cfg, database.DB()))
	g := rg.Group("/ops", middleware.Auth())
	{
		g.GET("/backups", middleware.RequirePerm("ops:list"), h.Backups)
		g.POST("/backups", middleware.RequirePerm("ops:backup"), h.RunBackup)
		g.DELETE("/backups", middleware.RequirePerm("ops:backup"), h.DeleteBackup)
		g.GET("/backups/download", middleware.RequirePerm("ops:backup"), h.DownloadBackup)

		g.POST("/restore", middleware.RequirePerm("ops:restore"), h.Restore)

		g.GET("/cleanup/preview", middleware.RequirePerm("ops:list"), h.CleanupPreview)
		g.POST("/cleanup/apply", middleware.RequirePerm("ops:cleanup"), h.CleanupApply)

		g.GET("/check", middleware.RequirePerm("ops:list"), h.Check)

		g.GET("/upgrade", middleware.RequirePerm("ops:list"), h.UpgradeStatus)
		g.GET("/upgrade/records", middleware.RequirePerm("ops:list"), h.UpgradeRecords)
		g.POST("/upgrade/package", middleware.RequirePerm("ops:upgrade"), h.UploadPackage)
		g.POST("/upgrade/backup", middleware.RequirePerm("ops:upgrade"), h.UpgradeBackup)
		g.POST("/upgrade/apply", middleware.RequirePerm("ops:upgrade"), h.ApplyUpgrade)
	}
}
