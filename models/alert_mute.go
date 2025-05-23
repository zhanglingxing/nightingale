package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/toolkits/pkg/logger"

	"github.com/pkg/errors"
)

type TagFilter struct {
	Key    string              `json:"key"`   // tag key
	Func   string              `json:"func"`  // `==` | `=~` | `in` | `!=` | `!~` | `not in`
	Op     string              `json:"op"`    // `==` | `=~` | `in` | `!=` | `!~` | `not in`
	Value  string              `json:"value"` // tag value
	Regexp *regexp.Regexp      // parse value to regexp if func = '=~' or '!~'
	Vset   map[string]struct{} // parse value to regexp if func = 'in' or 'not in'
}

func (t *TagFilter) Verify() error {
	if t.Key == "" {
		return errors.New("tag key cannot be empty")
	}

	if t.Func == "" {
		t.Func = t.Op
	}

	if t.Func != "==" && t.Func != "!=" && t.Func != "in" && t.Func != "not in" &&
		t.Func != "=~" && t.Func != "!~" {
		return errors.New("invalid operation")
	}

	return nil
}

func ParseTagFilter(bFilters []TagFilter) ([]TagFilter, error) {
	var err error
	for i := 0; i < len(bFilters); i++ {
		if bFilters[i].Func == "=~" || bFilters[i].Func == "!~" {
			bFilters[i].Regexp, err = regexp.Compile(bFilters[i].Value)
			if err != nil {
				return nil, err
			}
		} else if bFilters[i].Func == "in" || bFilters[i].Func == "not in" {
			arr := strings.Fields(bFilters[i].Value)
			bFilters[i].Vset = make(map[string]struct{})
			for j := 0; j < len(arr); j++ {
				bFilters[i].Vset[arr[j]] = struct{}{}
			}
		}
	}
	return bFilters, nil
}

func GetTagFilters(jsonArr ormx.JSONArr) ([]TagFilter, error) {
	if jsonArr == nil || len([]byte(jsonArr)) == 0 {
		return []TagFilter{}, nil
	}

	bFilters := make([]TagFilter, 0)
	err := json.Unmarshal(jsonArr, &bFilters)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(bFilters); i++ {
		if bFilters[i].Func == "=~" || bFilters[i].Func == "!~" {
			bFilters[i].Regexp, err = regexp.Compile(bFilters[i].Value)
			if err != nil {
				return nil, err
			}
		} else if bFilters[i].Func == "in" || bFilters[i].Func == "not in" {
			arr := strings.Fields(bFilters[i].Value)
			bFilters[i].Vset = make(map[string]struct{})
			for j := 0; j < len(arr); j++ {
				bFilters[i].Vset[arr[j]] = struct{}{}
			}
		}
	}

	return bFilters, nil
}

const TimeRange int = 0
const Periodic int = 1

type AlertMute struct {
	Id                int64          `json:"id" gorm:"primaryKey"`
	GroupId           int64          `json:"group_id"`
	Note              string         `json:"note"`
	Cate              string         `json:"cate"`
	Prod              string         `json:"prod"`
	DatasourceIds     string         `json:"-" gorm:"datasource_ids"` // datasource ids
	DatasourceIdsJson []int64        `json:"datasource_ids" gorm:"-"` // for fe
	Cluster           string         `json:"cluster"`                 // take effect by clusters, seperated by space
	Tags              ormx.JSONArr   `json:"tags"`
	Cause             string         `json:"cause"`
	Btime             int64          `json:"btime"`
	Etime             int64          `json:"etime"`
	Disabled          int            `json:"disabled"`           // 0: enabled, 1: disabled
	Activated         int            `json:"activated" gorm:"-"` // 0: not activated, 1: activated
	CreateBy          string         `json:"create_by"`
	UpdateBy          string         `json:"update_by"`
	CreateAt          int64          `json:"create_at"`
	UpdateAt          int64          `json:"update_at"`
	ITags             []TagFilter    `json:"-" gorm:"-"`     // inner tags
	MuteTimeType      int            `json:"mute_time_type"` //  0: mute by time range, 1: mute by periodic time
	PeriodicMutes     string         `json:"-" gorm:"periodic_mutes"`
	PeriodicMutesJson []PeriodicMute `json:"periodic_mutes" gorm:"-"`
	Severities        string         `json:"-" gorm:"severities"`
	SeveritiesJson    []int          `json:"severities" gorm:"-"`
}

type PeriodicMute struct {
	EnableStime      string `json:"enable_stime"`        // split by space: "00:00 10:00 12:00"
	EnableEtime      string `json:"enable_etime"`        // split by space: "00:00 10:00 12:00"
	EnableDaysOfWeek string `json:"enable_days_of_week"` // eg: "0 1 2 3 4 5 6"
}

func (m *AlertMute) TableName() string {
	return "alert_mute"
}

func AlertMuteGetById(ctx *ctx.Context, id int64) (*AlertMute, error) {
	return AlertMuteGet(ctx, "id=?", id)
}

func AlertMuteGet(ctx *ctx.Context, where string, args ...interface{}) (*AlertMute, error) {
	var lst []*AlertMute
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}
	err = lst[0].DB2FE()
	return lst[0], err
}

func AlertMuteGets(ctx *ctx.Context, prods []string, bgid int64, disabled int, query string) (lst []AlertMute, err error) {
	session := DB(ctx)

	if bgid != -1 {
		session = session.Where("group_id = ?", bgid)
	}

	if len(prods) > 0 {
		session = session.Where("prod in (?)", prods)
	}

	if disabled != -1 {
		if disabled == 0 {
			session = session.Where("disabled = 0")
		} else {
			session = session.Where("disabled = 1")
		}
	}

	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			qarg := "%" + arr[i] + "%"
			session = session.Where("cause like ?", qarg)
		}
	}

	err = session.Order("id desc").Find(&lst).Error
	for i := 0; i < len(lst); i++ {
		lst[i].DB2FE()
	}
	return
}

func AlertMuteGetsByBG(ctx *ctx.Context, groupId int64) (lst []AlertMute, err error) {
	err = DB(ctx).Where("group_id=?", groupId).Order("id desc").Find(&lst).Error
	for i := 0; i < len(lst); i++ {
		lst[i].DB2FE()
	}
	return
}

func AlertMuteGetsByBGIds(ctx *ctx.Context, bgids []int64) (lst []AlertMute, err error) {
	session := DB(ctx)
	if len(bgids) > 0 {
		session = session.Where("group_id in (?)", bgids)
	}

	err = session.Order("id desc").Find(&lst).Error
	for i := 0; i < len(lst); i++ {
		lst[i].DB2FE()
	}
	return
}

func (m *AlertMute) Verify() error {
	if m.GroupId < 0 {
		return errors.New("group_id invalid")
	}

	if IsAllDatasource(m.DatasourceIdsJson) {
		m.DatasourceIdsJson = []int64{0}
	}

	if m.Etime <= m.Btime {
		return fmt.Errorf("oops... etime(%d) <= btime(%d)", m.Etime, m.Btime)
	}

	if err := m.Parse(); err != nil {
		return err
	}

	return nil
}

func (m *AlertMute) Parse() error {
	var err error
	m.ITags, err = GetTagFilters(m.Tags)
	if err != nil {
		return err
	}

	return nil
}

func (m *AlertMute) Add(ctx *ctx.Context) error {
	if err := m.Verify(); err != nil {
		return err
	}

	if err := m.FE2DB(); err != nil {
		return err
	}

	now := time.Now().Unix()
	m.CreateAt = now
	m.UpdateAt = now
	return Insert(ctx, m)
}

func (m *AlertMute) Update(ctx *ctx.Context, arm AlertMute) error {

	arm.Id = m.Id
	arm.GroupId = m.GroupId
	arm.CreateAt = m.CreateAt
	arm.CreateBy = m.CreateBy
	arm.UpdateAt = time.Now().Unix()

	err := arm.Verify()
	if err != nil {
		return err
	}

	if err := arm.FE2DB(); err != nil {
		return err
	}

	return DB(ctx).Model(m).Select("*").Updates(arm).Error
}

func (m *AlertMute) FE2DB() error {
	idsBytes, err := json.Marshal(m.DatasourceIdsJson)
	if err != nil {
		return err
	}
	m.DatasourceIds = string(idsBytes)

	periodicMutesBytes, err := json.Marshal(m.PeriodicMutesJson)
	if err != nil {
		return err
	}
	m.PeriodicMutes = string(periodicMutesBytes)

	if len(m.SeveritiesJson) > 0 {
		severtiesBytes, err := json.Marshal(m.SeveritiesJson)
		if err != nil {
			return err
		}
		m.Severities = string(severtiesBytes)
	}

	return nil
}

func (m *AlertMute) DB2FE() error {
	err := json.Unmarshal([]byte(m.DatasourceIds), &m.DatasourceIdsJson)
	if err != nil {
		return err
	}

	if m.DatasourceIdsJson == nil {
		m.DatasourceIdsJson = []int64{}
	}

	err = json.Unmarshal([]byte(m.PeriodicMutes), &m.PeriodicMutesJson)
	if err != nil {
		return err
	}

	if m.Severities != "" {
		err = json.Unmarshal([]byte(m.Severities), &m.SeveritiesJson)
		if err != nil {
			return err
		}
	}

	// 检查时间范围
	isWithinTime := false
	if m.MuteTimeType == TimeRange {
		isWithinTime = m.IsWithinTimeRange(time.Now().Unix())
	} else if m.MuteTimeType == Periodic {
		isWithinTime = m.IsWithinPeriodicMute(time.Now().Unix())
	} else {
		logger.Warningf("mute time type invalid, %d", m.MuteTimeType)
	}

	if isWithinTime {
		m.Activated = 1
	} else {
		m.Activated = 0
	}

	return err
}

func (m *AlertMute) UpdateFieldsMap(ctx *ctx.Context, fields map[string]interface{}) error {
	return DB(ctx).Model(m).Updates(fields).Error
}

func (m *AlertMute) IsWithinTimeRange(checkTime int64) bool {
	if checkTime < m.Btime || checkTime > m.Etime {
		return false
	}
	return true
}

func (m *AlertMute) IsWithinPeriodicMute(checkTime int64) bool {
	tm := time.Unix(checkTime, 0)
	triggerTime := tm.Format("15:04")
	triggerWeek := strconv.Itoa(int(tm.Weekday()))

	for i := 0; i < len(m.PeriodicMutesJson); i++ {
		if strings.Contains(m.PeriodicMutesJson[i].EnableDaysOfWeek, triggerWeek) {
			if m.PeriodicMutesJson[i].EnableStime == m.PeriodicMutesJson[i].EnableEtime || (m.PeriodicMutesJson[i].EnableStime == "00:00" && m.PeriodicMutesJson[i].EnableEtime == "23:59") {
				return true
			} else if m.PeriodicMutesJson[i].EnableStime < m.PeriodicMutesJson[i].EnableEtime {
				if triggerTime >= m.PeriodicMutesJson[i].EnableStime && triggerTime < m.PeriodicMutesJson[i].EnableEtime {
					return true
				}
			} else {
				if triggerTime >= m.PeriodicMutesJson[i].EnableStime || triggerTime < m.PeriodicMutesJson[i].EnableEtime {
					return true
				}
			}
		}
	}

	return false
}

func AlertMuteDel(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB(ctx).Where("id in ?", ids).Delete(new(AlertMute)).Error
}

func AlertMuteStatistics(ctx *ctx.Context) (*Statistics, error) {
	var stats []*Statistics
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=alert_mute")
		return s, err
	}

	session := DB(ctx).Model(&AlertMute{}).Select("count(*) as total", "max(update_at) as last_updated")

	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func AlertMuteGetsAll(ctx *ctx.Context) ([]*AlertMute, error) {
	// get my cluster's mutes
	var lst []*AlertMute
	if !ctx.IsCenter {
		lst, err := poster.GetByUrls[[]*AlertMute](ctx, "/v1/n9e/alert-mutes?disabled=0")
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(lst); i++ {
			lst[i].FE2DB()
		}
		return lst, err
	}

	session := DB(ctx).Model(&AlertMute{}).Where("disabled = 0")

	err := session.Find(&lst).Error
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(lst); i++ {
		lst[i].DB2FE()
	}

	return lst, err
}

func AlertMuteUpgradeToV6(ctx *ctx.Context, dsm map[string]Datasource) error {
	var lst []*AlertMute
	err := DB(ctx).Find(&lst).Error
	if err != nil {
		return err
	}

	for i := 0; i < len(lst); i++ {
		var ids []int64
		if lst[i].Cluster == "$all" {
			ids = append(ids, 0)
		} else {
			clusters := strings.Fields(lst[i].Cluster)
			for j := 0; j < len(clusters); j++ {
				if ds, exists := dsm[clusters[j]]; exists {
					ids = append(ids, ds.Id)
				}
			}
		}

		b, err := json.Marshal(ids)
		if err != nil {
			continue
		}
		lst[i].DatasourceIds = string(b)

		err = lst[i].UpdateFieldsMap(ctx, map[string]interface{}{
			"datasource_ids": lst[i].DatasourceIds,
		})
		if err != nil {
			logger.Errorf("update alert rule:%d datasource ids failed, %v", lst[i].Id, err)
		}
	}
	return nil
}
