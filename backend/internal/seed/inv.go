package seed

import (
	"errors"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"nova-erp/internal/database"
	"nova-erp/internal/logger"
	"nova-erp/internal/model"
)

// seedUnit 是示例商品下的一个计量单位。
type seedUnit struct {
	unit           string
	rate           float64 // 1 该单位 = rate 个基本单位
	base           bool
	purchase, sale float64
}

// seedSpec 是示例商品下的一个规格（即一个库存桶）。
type seedSpec struct {
	spec             string
	stock, warnStock float64
	units            []seedUnit
}

// seedProduct 是示例商品档案（含规格与计量单位）。
type seedProduct struct {
	code, name, category string
	specs                []seedSpec
}

// seedLine 描述一条用于种子数据的明细：商品 / 规格 / 单位 三个下标 + 数量。
//
// 单位下标刻意允许指向非基本单位（如「根」「箱」），示例数据里必须有一条
// 换算订单，否则「1 根 = 6 米」这条链路在演示库里从没被走过。
type seedLine struct {
	product, spec, unit int
	qty                 float64
}

// demoProducts 是演示商品档案。覆盖三种典型形态：
// 单规格单单位（托盘）、多规格单单位（钢板 / 电机 / 编织袋）、
// 多规格多单位（钢管 / 轴承 / 润滑脂 / 钢丝绳）。
var demoProducts = []seedProduct{
	{
		code: "SP001", name: "热轧钢板", category: "钢材",
		specs: []seedSpec{
			{spec: "Q235B 6mm", stock: 120, warnStock: 40, units: []seedUnit{{unit: "张", rate: 1, base: true, purchase: 3200, sale: 3680}}},
			{spec: "Q235B 8mm", stock: 60, warnStock: 20, units: []seedUnit{{unit: "张", rate: 1, base: true, purchase: 4100, sale: 4700}}},
		},
	},
	{
		code: "SP002", name: "不锈钢管", category: "钢材",
		specs: []seedSpec{
			{
				spec: "304 φ32", stock: 2600, warnStock: 600,
				units: []seedUnit{
					{unit: "米", rate: 1, base: true, purchase: 18.5, sale: 21.5},
					{unit: "根", rate: 6, purchase: 111, sale: 129},
				},
			},
			{spec: "304 φ48", stock: 1200, warnStock: 300, units: []seedUnit{{unit: "米", rate: 1, base: true, purchase: 26, sale: 30}}},
		},
	},
	{
		code: "SP003", name: "深沟球轴承", category: "五金",
		specs: []seedSpec{
			{
				spec: "6206", stock: 600, warnStock: 100,
				units: []seedUnit{
					{unit: "个", rate: 1, base: true, purchase: 32, sale: 48},
					{unit: "盒", rate: 10, purchase: 310, sale: 460},
				},
			},
		},
	},
	{
		code: "SP004", name: "三相异步电机", category: "机电",
		specs: []seedSpec{
			{spec: "5.5kW", stock: 45, warnStock: 10, units: []seedUnit{{unit: "台", rate: 1, base: true, purchase: 1280, sale: 1580}}},
			{spec: "7.5kW", stock: 28, warnStock: 8, units: []seedUnit{{unit: "台", rate: 1, base: true, purchase: 1750, sale: 2100}}},
		},
	},
	{
		code: "SP005", name: "编织袋", category: "包装",
		specs: []seedSpec{
			{spec: "50kg", stock: 12000, warnStock: 2000, units: []seedUnit{{unit: "条", rate: 1, base: true, purchase: 1.2, sale: 2}}},
			{spec: "25kg", stock: 6000, warnStock: 1500, units: []seedUnit{{unit: "条", rate: 1, base: true, purchase: 0.9, sale: 1.5}}},
		},
	},
	{
		code: "SP006", name: "塑料托盘", category: "包装",
		specs: []seedSpec{
			{spec: "1200×1000", stock: 80, warnStock: 20, units: []seedUnit{{unit: "个", rate: 1, base: true, purchase: 165, sale: 210}}},
		},
	},
	{
		code: "SP007", name: "工业润滑脂", category: "化工",
		specs: []seedSpec{
			{
				// 库存 30 < 预警 50：演示库里必须留一条会触发预警的规格，
				// 否则「库存预警」这个标记在概览与库存页永远是 0，等于没验过。
				spec: "2#", stock: 30, warnStock: 50,
				units: []seedUnit{
					{unit: "桶", rate: 1, base: true, purchase: 240, sale: 320},
					{unit: "箱", rate: 4, purchase: 950, sale: 1260},
				},
			},
		},
	},
	{
		code: "SP008", name: "镀锌钢丝绳", category: "钢材",
		specs: []seedSpec{
			{
				spec: "12mm", stock: 1500, warnStock: 300,
				units: []seedUnit{
					{unit: "米", rate: 1, base: true, purchase: 9.5, sale: 13},
					{unit: "卷", rate: 100, purchase: 940, sale: 1290},
				},
			},
		},
	},
}

// demoCategories 是演示分类字典，顺序即排序。
var demoCategories = []string{"钢材", "五金", "机电", "包装", "化工"}

// demoProduct 是按商品下标取出的已落库档案，便于装配单据明细。
type demoProduct struct {
	product model.InvProduct
	specs   []model.InvProductSpec
	units   [][]model.InvProductUnit // 与 specs 同下标
}

// seedInv 写入进销存演示数据（分类 / 客户 / 供应商 / 商品 / 单据 / 出入库 / 打印模板）。
//
// 幂等：只要 inv_customers 已有数据就整体跳过。业务数据不像系统字典那样
// 需要「补齐缺失项」，重复灌入只会污染真实数据。
func seedInv(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.InvCustomer{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	now := time.Now().In(database.Location())
	// 单据日期落在最近 30 天内，保证报表的趋势图与本月汇总有数据；
	// 客户创建时间刻意分成「本月」与「上月」，让「本月新增」不是全量。
	dayAgo := func(n int) time.Time {
		d := now.AddDate(0, 0, -n)
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, now.Location())
	}
	monthAgo := func(n int) time.Time { return now.AddDate(0, -n, 0) }

	categories := make([]model.InvCategory, 0, len(demoCategories))
	for i, name := range demoCategories {
		categories = append(categories, model.InvCategory{Name: name, Sort: i, Status: 1})
	}
	if err := db.Create(&categories).Error; err != nil {
		return fmt.Errorf("写入商品分类失败: %w", err)
	}
	categoryID := map[string]int64{}
	for _, c := range categories {
		categoryID[c.Name] = c.ID
	}

	customers := []model.InvCustomer{
		{Code: "KH001", Name: "华东制造", Contact: "张伟", Phone: "13800006677", Address: "苏州工业园区星湖街218号", TaxNo: "91320594MA1W8KX40A", Status: 1, BaseModel: model.BaseModel{CreatedAt: monthAgo(5)}},
		{Code: "KH002", Name: "远大贸易", Contact: "李娜", Phone: "13900002231", Address: "郑州市金水区农业路72号", TaxNo: "91410105MA9FGB2X7C", Status: 1, BaseModel: model.BaseModel{CreatedAt: monthAgo(3)}},
		{Code: "KH003", Name: "精工科技", Contact: "王强", Phone: "13700008890", Address: "杭州市西湖区文三路198号", TaxNo: "91330106MA2HY7Q3XB", Status: 1, BaseModel: model.BaseModel{CreatedAt: monthAgo(2)}},
		{Code: "KH004", Name: "中原重工", Contact: "刘洋", Phone: "13500004412", Address: "信阳市浉河区工区路25号", TaxNo: "91411500MA9T4L8E1F", Status: 1, BaseModel: model.BaseModel{CreatedAt: now.AddDate(0, 0, -6)}},
		{Code: "KH005", Name: "鑫源物资", Contact: "陈静", Phone: "13600009087", Address: "济南市历下区解放路63号", TaxNo: "91370100MA3D9P5G2H", Status: 1, BaseModel: model.BaseModel{CreatedAt: now.AddDate(0, 0, -3)}},
	}
	if err := db.Create(&customers).Error; err != nil {
		return fmt.Errorf("写入客户失败: %w", err)
	}

	suppliers := []model.InvSupplier{
		{Code: "GYS001", Name: "宏达钢材", Contact: "王建国", Phone: "13900005510", Address: "郑州市管城回族区货栈街32号", TaxNo: "91410103MA9X2D4K5B", Status: 1},
		{Code: "GYS002", Name: "顺达物流", Contact: "赵敏", Phone: "13700003325", Address: "郑州市金水区经三路66号", TaxNo: "91410105MA40P7T9W2", Status: 1},
		{Code: "GYS003", Name: "中原机电", Contact: "孙立", Phone: "13500007802", Address: "郑州市二七区大学路18号", TaxNo: "91410102MA1TK8Q3E6", Status: 1},
		{Code: "GYS004", Name: "金桥包装", Contact: "周芳", Phone: "13800009146", Address: "郑州市高新区科学大道81号", TaxNo: "91410100MA3FB6H2X1", Status: 1},
		{Code: "GYS005", Name: "蓝天化工", Contact: "吴刚", Phone: "13600002287", Address: "洛阳市涧西区建设路45号", TaxNo: "91410305MA5GT9L4P8", Status: 1},
	}
	if err := db.Create(&suppliers).Error; err != nil {
		return fmt.Errorf("写入供应商失败: %w", err)
	}

	products, err := seedProducts(db, categoryID)
	if err != nil {
		return err
	}

	templates := []model.InvPrintTemplate{
		{Code: "BG01", Name: "采购订单", DocType: "采购订单", Paper: "A4", Status: 1},
		{Code: "BG02", Name: "采购入库单", DocType: "采购入库单", Paper: "A4", Status: 1},
		{Code: "BG03", Name: "采购退货单", DocType: "采购退货单", Paper: "A5", Status: 1},
		{Code: "BG04", Name: "销售订单", DocType: "销售订单", Paper: "A4", Status: 1},
		{Code: "BG05", Name: "销售出库单", DocType: "销售出库单", Paper: "A4", Status: 1},
		{Code: "BG06", Name: "销售退货单", DocType: "销售退货单", Paper: "A5", Status: 1},
		{Code: "BG07", Name: "客户对账单", DocType: "客户对账单", Paper: "A4", Status: 1},
		{Code: "BG08", Name: "库存盘点单", DocType: "库存盘点单", Paper: "A4", Status: 0, Remark: "已停用"},
	}
	if err := db.Create(&templates).Error; err != nil {
		return fmt.Errorf("写入打印模板失败: %w", err)
	}

	if err := seedInvOrders(db, customers, suppliers, products, dayAgo); err != nil {
		return err
	}
	if err := seedInvStockOrders(db, products, dayAgo); err != nil {
		return err
	}

	logger.Info("已初始化进销存演示数据",
		zap.Int("categories", len(categories)),
		zap.Int("customers", len(customers)),
		zap.Int("suppliers", len(suppliers)),
		zap.Int("products", len(products)),
	)
	return nil
}

// seedProducts 写入商品档案及其规格与计量单位，并回填各层 ID。
func seedProducts(db *gorm.DB, categoryID map[string]int64) ([]demoProduct, error) {
	out := make([]demoProduct, 0, len(demoProducts))
	for _, p := range demoProducts {
		product := model.InvProduct{
			Code: p.code, Name: p.name, CategoryID: categoryID[p.category], Status: 1,
		}
		if err := db.Create(&product).Error; err != nil {
			return nil, fmt.Errorf("写入商品 %s 失败: %w", p.code, err)
		}
		item := demoProduct{product: product}
		for si, sp := range p.specs {
			spec := model.InvProductSpec{
				ProductID: product.ID, Spec: sp.spec,
				Stock: sp.stock, WarnStock: sp.warnStock, Sort: si, Status: 1,
			}
			if err := db.Create(&spec).Error; err != nil {
				return nil, fmt.Errorf("写入商品 %s 的规格 %s 失败: %w", p.code, sp.spec, err)
			}
			unitRows := make([]model.InvProductUnit, 0, len(sp.units))
			for ui, u := range sp.units {
				unitRows = append(unitRows, model.InvProductUnit{
					SpecID: spec.ID, Unit: u.unit, Rate: u.rate, IsBase: u.base,
					PurchasePrice: u.purchase, SalePrice: u.sale, Sort: ui, Status: 1,
				})
			}
			if err := db.Create(&unitRows).Error; err != nil {
				return nil, fmt.Errorf("写入规格 %s 的计量单位失败: %w", sp.spec, err)
			}
			item.specs = append(item.specs, spec)
			item.units = append(item.units, unitRows)
		}
		out = append(out, item)
	}
	return out, nil
}

// buildOrderItem 按 商品/规格/单位 三个下标装配一条明细，含换算后的基本数量。
func buildOrderItem(products []demoProduct, biz string, ln seedLine) model.InvOrderItem {
	d := products[ln.product]
	spec := d.specs[ln.spec]
	unit := d.units[ln.spec][ln.unit]
	price := unit.SalePrice
	if biz == model.InvBizPurchase {
		price = unit.PurchasePrice
	}
	return model.InvOrderItem{
		ProductID: d.product.ID, ProductName: d.product.Name,
		SpecID: spec.ID, Spec: spec.Spec,
		UnitID: unit.ID, Unit: unit.Unit, Rate: unit.Rate,
		Qty: ln.qty, QtyBase: math.Round(ln.qty*unit.Rate*10000) / 10000,
		Price: price, Amount: math.Round(ln.qty*price*100) / 100,
	}
}

// seedInvProductTopUp 补齐演示档案里缺失的规格与计量单位。
//
// seedInv 的幂等口径是「客户表非空就整体跳过」。这对**单据**是对的 ——
// 重复灌入只会污染真实业务数据；但对**主数据字典**不合适：早期版本的库是由
// 老的单规格结构迁移过来的，每个商品只有一条「规格 + 基本单位」，
// 于是「1 根 = 6 米」这类多计量单位示例一条都看不到，演示时像是没做多单位。
//
// 所以这里单独走一条「按演示编码补缺」的路，规则：
//   - 只处理演示档案里声明的编码与规格名，绝不碰用户自建的商品；
//   - 同名规格 / 单位已存在就跳过，用户改过的价格与换算率一律保留；
//   - 演示商品被用户删掉了也不重建（那是他的选择，不是数据缺失）。
func seedInvProductTopUp(db *gorm.DB) error {
	for _, p := range demoProducts {
		var product model.InvProduct
		err := db.Where("code = ?", p.code).First(&product).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		for _, sp := range p.specs {
			var spec model.InvProductSpec
			err := db.Where("product_id = ? AND spec = ?", product.ID, sp.spec).First(&spec).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			for ui, u := range sp.units {
				var cnt int64
				if err := db.Model(&model.InvProductUnit{}).
					Where("spec_id = ? AND unit = ?", spec.ID, u.unit).Count(&cnt).Error; err != nil {
					return err
				}
				if cnt > 0 {
					continue
				}
				// 基本单位是库存的计数单位，一个规格只能有一个：
				// 规格已经有基本单位时再补一个，档案就直接不自洽了
				if u.base {
					var baseCnt int64
					if err := db.Model(&model.InvProductUnit{}).
						Where("spec_id = ? AND is_base", spec.ID).Count(&baseCnt).Error; err != nil {
						return err
					}
					if baseCnt > 0 {
						continue
					}
				}
				unit := model.InvProductUnit{
					SpecID: spec.ID, Unit: u.unit, Rate: u.rate, IsBase: u.base,
					PurchasePrice: u.purchase, SalePrice: u.sale, Sort: ui, Status: model.StatusEnabled,
				}
				if err := db.Create(&unit).Error; err != nil {
					return fmt.Errorf("补齐 %s 的计量单位 %s 失败: %w", p.code, u.unit, err)
				}
				logger.Info("已补齐演示档案的计量单位",
					zap.String("product", p.code),
					zap.String("spec", sp.spec),
					zap.String("unit", u.unit))
			}
		}
	}
	return nil
}

// seedInvOrders 生成采购 / 销售示例单据及其明细与部分结算记录。
func seedInvOrders(db *gorm.DB, customers []model.InvCustomer, suppliers []model.InvSupplier, products []demoProduct, dayAgo func(int) time.Time) error {
	type spec struct {
		biz      string
		partyIdx int
		day      int
		dueDays  int
		status   int16
		settle   float64 // 已结算金额占总额的比例（0~1）
		lines    []seedLine
	}
	specs := []spec{
		// 销售单
		{model.InvBizSales, 0, 28, 30, model.InvStatusDone, 1, []seedLine{{0, 0, 0, 20}, {1, 0, 0, 300}}},
		{model.InvBizSales, 1, 24, 30, model.InvStatusDone, 0.5, []seedLine{{2, 0, 0, 200}, {3, 0, 0, 5}}},
		{model.InvBizSales, 2, 20, 15, model.InvStatusDone, 0, []seedLine{{4, 0, 0, 3000}}},
		{model.InvBizSales, 0, 16, 30, model.InvStatusDone, 1, []seedLine{{5, 0, 0, 30}, {3, 0, 0, 4}}},
		{model.InvBizSales, 3, 12, 30, model.InvStatusDone, 0.3, []seedLine{{0, 0, 0, 15}, {2, 0, 0, 150}}},
		{model.InvBizSales, 4, 8, 30, model.InvStatusDone, 0, []seedLine{{6, 0, 0, 20}, {7, 0, 0, 400}}},
		// 同一张单子上同时出现「根」与「条」：演示多单位并存的单据长什么样
		{model.InvBizSales, 2, 4, 30, model.InvStatusDone, 1, []seedLine{{1, 0, 1, 10}, {4, 0, 0, 1500}}},
		{model.InvBizSales, 1, 1, 0, model.InvStatusPending, 0, []seedLine{{0, 0, 0, 10}, {6, 0, 0, 15}}}, // dueDays=0：未填到期日
		// 采购单
		{model.InvBizPurchase, 0, 26, 15, model.InvStatusDone, 1, []seedLine{{0, 0, 0, 50}, {1, 0, 0, 1000}}},
		{model.InvBizPurchase, 2, 22, 15, model.InvStatusDone, 0.6, []seedLine{{3, 1, 0, 20}, {2, 0, 0, 300}}},
		{model.InvBizPurchase, 3, 17, 30, model.InvStatusDone, 0, []seedLine{{4, 0, 0, 5000}, {5, 0, 0, 100}}},
		// 「15 箱 = 60 桶」：采购按箱进，账上按桶记
		{model.InvBizPurchase, 4, 10, 30, model.InvStatusDone, 0, []seedLine{{6, 0, 1, 15}, {7, 0, 0, 800}}},
		{model.InvBizPurchase, 0, 5, 15, model.InvStatusDone, 1, []seedLine{{1, 1, 0, 600}}},
		{model.InvBizPurchase, 2, 2, 30, model.InvStatusPending, 0, []seedLine{{0, 0, 0, 30}}},
	}

	for i, sp := range specs {
		date := dayAgo(sp.day)
		prefix := "XS"
		partyID, partyName := customers[sp.partyIdx].ID, customers[sp.partyIdx].Name
		if sp.biz == model.InvBizPurchase {
			prefix = "CG"
			partyID, partyName = suppliers[sp.partyIdx].ID, suppliers[sp.partyIdx].Name
		}
		var total float64
		items := make([]model.InvOrderItem, 0, len(sp.lines))
		for _, ln := range sp.lines {
			it := buildOrderItem(products, sp.biz, ln)
			total += it.Amount
			items = append(items, it)
		}
		total = math.Round(total*100) / 100
		settled := math.Round(total*sp.settle*100) / 100
		// 到期日是选填：dueDays 为 0 时留空（落库 NULL）。
		// 示例数据刻意保留一张「未填到期日」的单据 —— 真实业务里这种单子很常见，
		// 演示数据全覆盖上它，逾期汇总与打印纸面的边界才有数据可验。
		var dueDate *time.Time
		if sp.dueDays > 0 {
			d := date.AddDate(0, 0, sp.dueDays)
			dueDate = &d
		}
		order := model.InvOrder{
			OrderNo:     fmt.Sprintf("%s%s%04d", prefix, date.Format("060102"), i+1),
			BizType:     sp.biz,
			PartyID:     partyID,
			PartyName:   partyName,
			OrderDate:   date,
			DueDate:     dueDate,
			Status:      sp.status,
			TotalAmount: total, SettledAmount: settled,
			CreatedBy: 1,
			BaseModel: model.BaseModel{CreatedAt: date},
		}
		if err := db.Create(&order).Error; err != nil {
			return fmt.Errorf("写入单据失败: %w", err)
		}
		for j := range items {
			items[j].OrderID = order.ID
		}
		if err := db.Create(&items).Error; err != nil {
			return fmt.Errorf("写入单据明细失败: %w", err)
		}
		if settled > 0 {
			rec := model.InvSettlement{
				OrderID: order.ID, BizType: sp.biz, PartyID: partyID, Amount: settled,
				Method: "银行转账", SettledAt: date.AddDate(0, 0, 3), CreatedBy: 1,
				BaseModel: model.BaseModel{CreatedAt: date.AddDate(0, 0, 3)},
			}
			if err := db.Create(&rec).Error; err != nil {
				return fmt.Errorf("写入结算记录失败: %w", err)
			}
		}
	}
	return nil
}

// seedInvStockOrders 生成若干手工出入库单据（库存页的历史记录）。
func seedInvStockOrders(db *gorm.DB, products []demoProduct, dayAgo func(int) time.Time) error {
	type spec struct {
		dir   string
		day   int
		lines []seedLine
	}
	specs := []spec{
		{model.InvStockIn, 27, []seedLine{{0, 0, 0, 50}, {1, 0, 0, 1000}}},
		{model.InvStockIn, 21, []seedLine{{3, 0, 0, 20}, {2, 0, 0, 300}}},
		{model.InvStockOut, 14, []seedLine{{4, 0, 0, 2000}, {5, 0, 0, 40}}},
		{model.InvStockIn, 9, []seedLine{{6, 0, 0, 40}, {7, 0, 0, 500}}},
		{model.InvStockOut, 5, []seedLine{{0, 0, 0, 12}, {2, 0, 0, 80}}},
		{model.InvStockOut, 2, []seedLine{{1, 0, 1, 20}}},
	}
	for i, sp := range specs {
		date := dayAgo(sp.day)
		order := model.InvStockOrder{
			OrderNo:    fmt.Sprintf("CK%s%04d", date.Format("060102"), i+1),
			Direction:  sp.dir,
			SourceType: model.InvSourceManual,
			OrderDate:  date, Status: 1, CreatedBy: 1,
			BaseModel: model.BaseModel{CreatedAt: date},
		}
		if err := db.Create(&order).Error; err != nil {
			return fmt.Errorf("写入出入库单失败: %w", err)
		}
		items := make([]model.InvStockItem, 0, len(sp.lines))
		for _, ln := range sp.lines {
			d := products[ln.product]
			specRow := d.specs[ln.spec]
			unit := d.units[ln.spec][ln.unit]
			items = append(items, model.InvStockItem{
				StockOrderID: order.ID, ProductID: d.product.ID, ProductName: d.product.Name,
				SpecID: specRow.ID, Spec: specRow.Spec,
				UnitID: unit.ID, Unit: unit.Unit, Rate: unit.Rate,
				Qty: ln.qty, QtyBase: math.Round(ln.qty*unit.Rate*10000) / 10000,
			})
		}
		if err := db.Create(&items).Error; err != nil {
			return fmt.Errorf("写入出入库明细失败: %w", err)
		}
	}
	return nil
}
