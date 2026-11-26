package command

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/url"
	"os"
	"strconv"
	"time"

	"kasinlo-cli/internal/client"
	"kasinlo-cli/internal/credential"
)

type queryFilter struct {
	flag  string
	key   string
	usage string
}

type readCommand struct {
	path        string
	idFlag      string
	unpaginated bool
	noKeyword   bool
	filters     []queryFilter
	boolFilters []queryFilter
}

var (
	partyFilters = []queryFilter{{"status", "status", "party status (0 or 1)"}}
	orderFilters = []queryFilter{
		{"status", "status", "order status"},
		{"settle-status", "settleStatus", "unpaid, partial or paid"},
		{"start-date", "startDate", "start date, inclusive"},
		{"end-date", "endDate", "end date, inclusive"},
	}
	financeFilters = []queryFilter{
		{"settle-status", "settle", "unpaid, partial, paid or overdue"},
		{"source", "source", "order or plan"},
	}

	contractFilters = []queryFilter{
		{"biz-type", "bizType", "purchase or sales"},
		{"status", "status", "1 active, 2 finished, 3 terminated"},
		{"stage", "stage", "unaudited, pending, accepted, warranty or warranty_due"},
		{"party-id", "partyId", "party ID"},
		{"start-date", "startDate", "sign date start, inclusive"},
		{"end-date", "endDate", "sign date end, inclusive"},
	}

	readCommands = map[string]readCommand{
		"contract list":   {path: "contracts", filters: contractFilters},
		"contract report": {path: "contract-report", filters: contractFilters, unpaginated: true},
		"contract ledger": {path: "contract-ledger", unpaginated: true, filters: []queryFilter{
			{"biz-type", "bizType", "purchase or sales"},
			{"status", "status", "1 active, 2 finished, 3 terminated"},
			{"from", "from", "settlement date start, inclusive"},
			{"to", "to", "settlement date end, inclusive"},
			{"limit", "limit", "maximum rows, 1 to 1000"},
		}},
		"contract get":         {path: "contracts", idFlag: "id"},
		"contract activity":    {path: "contract-activities", idFlag: "id"},
		"contract settlements": {path: "contract-plan-settlements", idFlag: "plan-id"},
		"report summary": {path: "business-summary", unpaginated: true, noKeyword: true, filters: []queryFilter{
			{"range", "range", "today, week, month, quarter or year"},
			{"from", "from", "start date, inclusive (overrides range)"},
			{"to", "to", "end date, inclusive"},
			{"category-id", "categoryId", "category ID, including descendants"},
		}},
		"inventory ranking": {path: "stock-ranking", unpaginated: true, noKeyword: true, filters: []queryFilter{
			{"limit", "limit", "maximum rows, 1 to 1000"},
		}},
		"inventory report": {path: "inventory-report", unpaginated: true, noKeyword: true},
		"inventory list":   {path: "inventory"},
		"category list": {path: "product-categories", filters: []queryFilter{
			{"status", "status", "category status (0 or 1)"},
			{"parent-id", "parentId", "direct parent category ID (0 for roots)"},
		}},
		"category get": {path: "product-categories", idFlag: "id"},
		"product list": {path: "products", filters: []queryFilter{
			{"status", "status", "product status"},
			{"category-id", "categoryId", "product category ID"},
		}},
		"product get":   {path: "products", idFlag: "id"},
		"customer list": {path: "customers", filters: partyFilters},
		"customer get":  {path: "customers", idFlag: "id"},
		"supplier list": {path: "suppliers", filters: partyFilters},
		"supplier get":  {path: "suppliers", idFlag: "id"},
		"purchase list": {path: "purchase-orders", filters: orderFilters},
		"purchase get":  {path: "purchase-orders", idFlag: "id"},
		"sales list":    {path: "sales-orders", filters: orderFilters},
		"sales get":     {path: "sales-orders", idFlag: "id"},
		"stock list": {
			path: "stock-orders",
			filters: []queryFilter{
				{"direction", "direction", "in or out"},
				{"source-type", "sourceType", "purchase or sales"},
				{"source-id", "sourceId", "source order ID"},
			},
			boolFilters: []queryFilter{{"include-voided", "includeVoided", "include voided stock orders"}},
		},
		"stock get":           {path: "stock-orders", idFlag: "id"},
		"finance receivables": {path: "receivables", filters: financeFilters},
		"finance payables":    {path: "payables", filters: financeFilters},
		"finance settlements": {path: "order-settlements", idFlag: "order-id"},
	}
)

func (cmd readCommand) run(ctx context.Context, api *client.Client, store credential.Store, name string, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := cmd.path
	var query url.Values
	if cmd.idFlag != "" {
		id := flags.Int64(cmd.idFlag, 0, "record ID")
		if err := flags.Parse(args); err != nil {
			return usageError(err.Error())
		}
		if flags.NArg() != 0 || *id <= 0 {
			return usageError("use --" + cmd.idFlag + " with a positive record ID")
		}
		path += "/" + strconv.FormatInt(*id, 10)
	} else {
		var err error
		query, err = cmd.listQuery(flags, args)
		if err != nil {
			return err
		}
	}

	current, err := store.Credential()
	if errors.Is(err, os.ErrNotExist) || err == nil && !time.Now().Before(current.ExpiresAt) {
		return &problem{"not_logged_in", "run kas auth login before querying " + path, 3}
	}
	if err != nil {
		return err
	}
	data, err := api.Read(ctx, current.Token, path, query)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(data)
}

func (cmd readCommand) listQuery(flags *flag.FlagSet, args []string) (url.Values, error) {
	var keyword string
	if !cmd.noKeyword {
		flags.StringVar(&keyword, "keyword", "", "search term")
	}
	page, size := 1, 10
	if !cmd.unpaginated {
		flags.IntVar(&page, "page", page, "page number")
		flags.IntVar(&size, "page-size", size, "page size")
	}
	filters := make(map[string]*string, len(cmd.filters))
	for _, filter := range cmd.filters {
		filters[filter.key] = flags.String(filter.flag, "", filter.usage)
	}
	boolFilters := make(map[string]*bool, len(cmd.boolFilters))
	for _, filter := range cmd.boolFilters {
		boolFilters[filter.key] = flags.Bool(filter.flag, false, filter.usage)
	}

	if err := flags.Parse(args); err != nil {
		return nil, usageError(err.Error())
	}
	if flags.NArg() != 0 || len(keyword) > 500 {
		return nil, usageError("use named flags and a keyword up to 500 bytes")
	}
	if page < 1 || size < 1 || size > 200 {
		return nil, usageError("use a positive page, page-size between 1 and 200, and keyword up to 500 bytes")
	}
	query := url.Values{}
	if !cmd.noKeyword {
		query.Set("keyword", keyword)
	}
	if !cmd.unpaginated {
		query.Set("page", strconv.Itoa(page))
		query.Set("pageSize", strconv.Itoa(size))
	}
	for key, value := range filters {
		if *value != "" {
			query.Set(key, *value)
		}
	}
	for key, enabled := range boolFilters {
		if *enabled {
			query.Set(key, "true")
		}
	}
	return query, nil
}
