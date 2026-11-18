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

	readCommands = map[string]readCommand{
		"inventory list": {path: "inventory"},
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
	keyword := flags.String("keyword", "", "search term")
	page := flags.Int("page", 1, "page number")
	size := flags.Int("page-size", 10, "page size")
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
	if flags.NArg() != 0 || *page < 1 || *size < 1 || *size > 200 || len(*keyword) > 500 {
		return nil, usageError("use a positive page, page-size between 1 and 200, and keyword up to 500 bytes")
	}
	query := url.Values{"page": {strconv.Itoa(*page)}, "pageSize": {strconv.Itoa(*size)}, "keyword": {*keyword}}
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
