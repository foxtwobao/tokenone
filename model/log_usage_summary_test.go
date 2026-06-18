package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLogUsageSummaryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := DB
	previousLogDB := LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	initCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&Log{}))

	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		common.RedisEnabled = previousRedisEnabled
		initCol()

		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func seedUsageSummaryLog(t *testing.T, db *gorm.DB, log Log) {
	t.Helper()
	require.NoError(t, db.Create(&log).Error)
}

func TestGetSelfDailyUsageSummaryAggregatesByToken(t *testing.T) {
	db := setupLogUsageSummaryTestDB(t)
	dayStart := int64(1717200000)
	dayEnd := dayStart + 86399

	seedUsageSummaryLog(t, db, Log{
		UserId:           10,
		Username:         "alice",
		CreatedAt:        dayStart + 100,
		Type:             LogTypeConsume,
		TokenId:          101,
		TokenName:        "prod-key",
		PromptTokens:     100,
		CompletionTokens: 30,
		Quota:            11,
		Other:            `{"cache_tokens":20,"cache_creation_tokens":5}`,
	})
	seedUsageSummaryLog(t, db, Log{
		UserId:           10,
		Username:         "alice",
		CreatedAt:        dayStart + 200,
		Type:             LogTypeConsume,
		TokenId:          101,
		TokenName:        "prod-key-renamed",
		PromptTokens:     50,
		CompletionTokens: 15,
		Quota:            7,
		Other:            `{"cache_tokens":10,"cache_creation_tokens_5m":3,"cache_creation_tokens_1h":2}`,
	})
	seedUsageSummaryLog(t, db, Log{
		UserId:           10,
		Username:         "alice",
		CreatedAt:        dayStart + 300,
		Type:             LogTypeConsume,
		TokenId:          102,
		TokenName:        "test-key",
		PromptTokens:     40,
		CompletionTokens: 9,
		Quota:            4,
		Other:            `{}`,
	})
	seedUsageSummaryLog(t, db, Log{
		UserId:           20,
		Username:         "bob",
		CreatedAt:        dayStart + 400,
		Type:             LogTypeConsume,
		TokenId:          201,
		TokenName:        "other-user",
		PromptTokens:     999,
		CompletionTokens: 999,
		Quota:            999,
	})
	seedUsageSummaryLog(t, db, Log{
		UserId:           10,
		Username:         "alice",
		CreatedAt:        dayEnd + 1,
		Type:             LogTypeConsume,
		TokenId:          101,
		TokenName:        "outside-range",
		PromptTokens:     999,
		CompletionTokens: 999,
		Quota:            999,
	})

	summary, err := GetSelfDailyUsageSummary(UsageSummaryQuery{
		UserId:         10,
		StartTimestamp: dayStart,
		EndTimestamp:   dayEnd,
	})

	require.NoError(t, err)
	require.Len(t, summary.Items, 2)
	assert.Equal(t, int64(3), summary.Total.RequestCount)
	assert.Equal(t, int64(190), summary.Total.InputTokens)
	assert.Equal(t, int64(40), summary.Total.CacheTokens)
	assert.Equal(t, int64(54), summary.Total.OutputTokens)
	assert.Equal(t, int64(22), summary.Total.Quota)

	assert.Equal(t, 101, summary.Items[0].TokenId)
	assert.Equal(t, "prod-key-renamed", summary.Items[0].TokenName)
	assert.Equal(t, int64(2), summary.Items[0].RequestCount)
	assert.Equal(t, int64(150), summary.Items[0].InputTokens)
	assert.Equal(t, int64(40), summary.Items[0].CacheTokens)
	assert.Equal(t, int64(45), summary.Items[0].OutputTokens)
	assert.Equal(t, int64(18), summary.Items[0].Quota)

	assert.Equal(t, 102, summary.Items[1].TokenId)
	assert.Equal(t, "test-key", summary.Items[1].TokenName)
	assert.Equal(t, int64(1), summary.Items[1].RequestCount)
}

func TestGetAdminDailyUsageSummaryAggregatesByUser(t *testing.T) {
	db := setupLogUsageSummaryTestDB(t)
	dayStart := int64(1717200000)
	dayEnd := dayStart + 86399

	seedUsageSummaryLog(t, db, Log{
		UserId:           10,
		Username:         "alice",
		CreatedAt:        dayStart + 100,
		Type:             LogTypeConsume,
		TokenId:          101,
		TokenName:        "alice-key",
		PromptTokens:     100,
		CompletionTokens: 30,
		Quota:            11,
		Other:            `{"cache_tokens":20}`,
	})
	seedUsageSummaryLog(t, db, Log{
		UserId:           20,
		Username:         "bob",
		CreatedAt:        dayStart + 200,
		Type:             LogTypeConsume,
		TokenId:          201,
		TokenName:        "bob-key",
		PromptTokens:     60,
		CompletionTokens: 15,
		Quota:            6,
		Other:            `{"cache_creation_tokens":4}`,
	})
	seedUsageSummaryLog(t, db, Log{
		UserId:           20,
		Username:         "bob",
		CreatedAt:        dayStart + 300,
		Type:             LogTypeConsume,
		TokenId:          202,
		TokenName:        "bob-second-key",
		PromptTokens:     40,
		CompletionTokens: 10,
		Quota:            5,
		Other:            `{"cache_tokens":3}`,
	})

	summary, err := GetAdminDailyUsageSummary(UsageSummaryQuery{
		StartTimestamp: dayStart,
		EndTimestamp:   dayEnd,
	})

	require.NoError(t, err)
	require.Len(t, summary.Items, 2)
	assert.Equal(t, int64(3), summary.Total.RequestCount)
	assert.Equal(t, int64(200), summary.Total.InputTokens)
	assert.Equal(t, int64(27), summary.Total.CacheTokens)
	assert.Equal(t, int64(55), summary.Total.OutputTokens)
	assert.Equal(t, int64(22), summary.Total.Quota)

	assert.Equal(t, 20, summary.Items[0].UserId)
	assert.Equal(t, "bob", summary.Items[0].Username)
	assert.Equal(t, int64(2), summary.Items[0].RequestCount)
	assert.Equal(t, int64(100), summary.Items[0].InputTokens)
	assert.Equal(t, int64(7), summary.Items[0].CacheTokens)
	assert.Equal(t, int64(25), summary.Items[0].OutputTokens)
	assert.Equal(t, int64(11), summary.Items[0].Quota)

	assert.Equal(t, 10, summary.Items[1].UserId)
	assert.Equal(t, "alice", summary.Items[1].Username)
	assert.Equal(t, int64(1), summary.Items[1].RequestCount)
}
