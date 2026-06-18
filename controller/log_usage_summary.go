package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetDailyUsageSummary(c *gin.Context) {
	query := buildUsageSummaryQuery(c, true)
	summary, err := model.GetAdminDailyUsageSummary(query)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func GetSelfDailyUsageSummary(c *gin.Context) {
	query := buildUsageSummaryQuery(c, false)
	query.UserId = c.GetInt("id")
	summary, err := model.GetSelfDailyUsageSummary(query)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func buildUsageSummaryQuery(c *gin.Context, admin bool) model.UsageSummaryQuery {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	startTimestamp, endTimestamp = normalizeUsageSummaryTimeRange(startTimestamp, endTimestamp)
	channel := 0
	if admin {
		channel, _ = strconv.Atoi(c.Query("channel"))
	}
	return model.UsageSummaryQuery{
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ModelName:      c.Query("model_name"),
		Username:       c.Query("username"),
		TokenName:      c.Query("token_name"),
		Channel:        channel,
		Group:          c.Query("group"),
	}
}

func normalizeUsageSummaryTimeRange(startTimestamp int64, endTimestamp int64) (int64, int64) {
	if startTimestamp == 0 && endTimestamp == 0 {
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return start.Unix(), start.AddDate(0, 0, 1).Add(-time.Second).Unix()
	}
	if startTimestamp != 0 && endTimestamp == 0 {
		return startTimestamp, time.Unix(startTimestamp, 0).AddDate(0, 0, 1).Add(-time.Second).Unix()
	}
	if startTimestamp == 0 && endTimestamp != 0 {
		end := time.Unix(endTimestamp, 0)
		start := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())
		return start.Unix(), endTimestamp
	}
	return startTimestamp, endTimestamp
}
