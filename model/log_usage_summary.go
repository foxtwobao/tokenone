package model

import (
	"errors"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

type UsageSummaryQuery struct {
	UserId         int
	StartTimestamp int64
	EndTimestamp   int64
	ModelName      string
	Username       string
	TokenName      string
	Channel        int
	Group          string
}

type UsageSummaryTotals struct {
	RequestCount int64 `json:"request_count"`
	InputTokens  int64 `json:"input_tokens"`
	CacheTokens  int64 `json:"cache_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
	Quota        int64 `json:"quota"`
}

type UsageSummaryItem struct {
	UserId       int    `json:"user_id,omitempty"`
	Username     string `json:"username,omitempty"`
	TokenId      int    `json:"token_id,omitempty"`
	TokenName    string `json:"token_name,omitempty"`
	RequestCount int64  `json:"request_count"`
	InputTokens  int64  `json:"input_tokens"`
	CacheTokens  int64  `json:"cache_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	Quota        int64  `json:"quota"`
}

type UsageSummaryResponse struct {
	Total UsageSummaryTotals `json:"total"`
	Items []UsageSummaryItem `json:"items"`
}

type usageSummaryLogRow struct {
	UserId           int
	Username         string
	TokenId          int
	TokenName        string
	PromptTokens     int
	CompletionTokens int
	Quota            int
	Other            string
}

func GetSelfDailyUsageSummary(query UsageSummaryQuery) (UsageSummaryResponse, error) {
	if query.UserId == 0 {
		return UsageSummaryResponse{}, errors.New("无效的用户")
	}
	return getDailyUsageSummary(query, false)
}

func GetAdminDailyUsageSummary(query UsageSummaryQuery) (UsageSummaryResponse, error) {
	return getDailyUsageSummary(query, true)
}

func getDailyUsageSummary(query UsageSummaryQuery, admin bool) (UsageSummaryResponse, error) {
	rows, err := findUsageSummaryRows(query, admin)
	if err != nil {
		common.SysError("failed to query daily usage summary: " + err.Error())
		return UsageSummaryResponse{}, errors.New("查询用量汇总失败")
	}

	items := make(map[string]*UsageSummaryItem)
	response := UsageSummaryResponse{
		Items: []UsageSummaryItem{},
	}
	for _, row := range rows {
		key := usageSummaryGroupKey(row, admin)
		item, ok := items[key]
		if !ok {
			item = &UsageSummaryItem{}
			if admin {
				item.UserId = row.UserId
				item.Username = row.Username
			} else {
				item.TokenId = row.TokenId
				item.TokenName = row.TokenName
			}
			items[key] = item
		}

		if !admin && row.TokenName != "" {
			item.TokenName = row.TokenName
		}
		if admin && row.Username != "" {
			item.Username = row.Username
		}

		inputTokens := int64(row.PromptTokens)
		outputTokens := int64(row.CompletionTokens)
		cacheTokens := usageSummaryCacheTokens(row.Other)
		quota := int64(row.Quota)

		item.RequestCount++
		item.InputTokens += inputTokens
		item.CacheTokens += cacheTokens
		item.OutputTokens += outputTokens
		item.TotalTokens += inputTokens + outputTokens
		item.Quota += quota

		response.Total.RequestCount++
		response.Total.InputTokens += inputTokens
		response.Total.CacheTokens += cacheTokens
		response.Total.OutputTokens += outputTokens
		response.Total.TotalTokens += inputTokens + outputTokens
		response.Total.Quota += quota
	}

	response.Items = make([]UsageSummaryItem, 0, len(items))
	for _, item := range items {
		response.Items = append(response.Items, *item)
	}
	sortUsageSummaryItems(response.Items, admin)
	return response, nil
}

func findUsageSummaryRows(query UsageSummaryQuery, admin bool) ([]usageSummaryLogRow, error) {
	tx := LOG_DB.Model(&Log{}).
		Select("user_id, username, token_id, token_name, prompt_tokens, completion_tokens, quota, other").
		Where("type = ?", LogTypeConsume)
	var err error
	if !admin {
		tx = tx.Where("user_id = ?", query.UserId)
	} else if query.UserId != 0 {
		tx = tx.Where("user_id = ?", query.UserId)
	}
	if query.StartTimestamp != 0 {
		tx = tx.Where("created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp != 0 {
		tx = tx.Where("created_at <= ?", query.EndTimestamp)
	}
	if tx, err = applyExplicitLogTextFilter(tx, "model_name", query.ModelName); err != nil {
		return nil, err
	}
	if admin {
		if tx, err = applyExplicitLogTextFilter(tx, "username", query.Username); err != nil {
			return nil, err
		}
	}
	if query.TokenName != "" {
		tx = tx.Where("token_name = ?", query.TokenName)
	}
	if query.Channel != 0 {
		tx = tx.Where("channel_id = ?", query.Channel)
	}
	if query.Group != "" {
		tx = tx.Where(logGroupCol+" = ?", query.Group)
	}

	var rows []usageSummaryLogRow
	return rows, tx.Find(&rows).Error
}

func usageSummaryGroupKey(row usageSummaryLogRow, admin bool) string {
	if admin {
		return strconv.Itoa(row.UserId)
	}
	return strconv.Itoa(row.TokenId)
}

func sortUsageSummaryItems(items []UsageSummaryItem, admin bool) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Quota != items[j].Quota {
			return items[i].Quota > items[j].Quota
		}
		if items[i].RequestCount != items[j].RequestCount {
			return items[i].RequestCount > items[j].RequestCount
		}
		if admin {
			return items[i].UserId < items[j].UserId
		}
		return items[i].TokenId < items[j].TokenId
	})
}

func usageSummaryCacheTokens(other string) int64 {
	if other == "" {
		return 0
	}
	var data map[string]interface{}
	if err := common.UnmarshalJsonStr(other, &data); err != nil {
		return 0
	}
	readTokens := usageSummaryNumber(data["cache_tokens"])
	writeTokens, ok := usageSummaryOptionalNumber(data["cache_write_tokens"])
	if !ok {
		write5m := usageSummaryNumber(data["cache_creation_tokens_5m"])
		write1h := usageSummaryNumber(data["cache_creation_tokens_1h"])
		if write5m > 0 || write1h > 0 {
			writeTokens = write5m + write1h
		} else {
			writeTokens = usageSummaryNumber(data["cache_creation_tokens"])
		}
	}
	return readTokens + writeTokens
}

func usageSummaryOptionalNumber(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case float64:
		return int64(v), true
	case int:
		return int64(v), true
	case int64:
		return v, true
	case int32:
		return int64(v), true
	case uint:
		return int64(v), true
	case uint64:
		return int64(v), true
	case uint32:
		return int64(v), true
	default:
		return 0, false
	}
}

func usageSummaryNumber(value interface{}) int64 {
	n, _ := usageSummaryOptionalNumber(value)
	return n
}
