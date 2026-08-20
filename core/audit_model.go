package core

import "github.com/arief-fajri/pgbase/tools/types"

var (
	_ Model = (*Audit)(nil)
	_ Model = (*AuditRead)(nil)
)

const (
	AuditsTableName     = "_audits"
	AuditReadsTableName = "_audit_reads"
)

// Audit is the write-trail (create/update/delete) audit model backed by the
// raw partitioned "_audits" table in the MAIN database.
type Audit struct {
	BaseModel

	CollectionName string             `db:"collection_name" json:"collection_name"`
	RecordId       string             `db:"record_id" json:"record_id"`
	Event          string             `db:"event" json:"event"`
	AuthId         string             `db:"auth_id" json:"auth_id"`
	AuthCollection string             `db:"auth_collection" json:"auth_collection"`
	Source         string             `db:"source" json:"source"`
	Changes        types.JSONMap[any] `db:"changes" json:"changes"`
	Snapshot       types.JSONMap[any] `db:"snapshot" json:"snapshot"`
	UserIP         string             `db:"user_ip" json:"user_ip"`
	UserAgent      string             `db:"user_agent" json:"user_agent"`
	Created        types.DateTime     `db:"created" json:"created"`
}

func (m *Audit) TableName() string {
	return AuditsTableName
}

// AuditRead is the read-trail (view/list) audit model backed by the raw
// partitioned "_audit_reads" table in the MAIN database. It stores only
// who/when/collection/filter metadata, never record content.
type AuditRead struct {
	BaseModel

	CollectionName string         `db:"collection_name" json:"collection_name"`
	RecordId       string         `db:"record_id" json:"record_id"`
	Event          string         `db:"event" json:"event"`
	AuthId         string         `db:"auth_id" json:"auth_id"`
	AuthCollection string         `db:"auth_collection" json:"auth_collection"`
	Source         string         `db:"source" json:"source"`
	Filter         string         `db:"filter" json:"filter"`
	Sort           string         `db:"sort" json:"sort"`
	Page           int            `db:"page" json:"page"`
	PerPage        int            `db:"per_page" json:"per_page"`
	TotalItems     int            `db:"total_items" json:"total_items"`
	UserIP         string         `db:"user_ip" json:"user_ip"`
	UserAgent      string         `db:"user_agent" json:"user_agent"`
	Created        types.DateTime `db:"created" json:"created"`
}

func (m *AuditRead) TableName() string {
	return AuditReadsTableName
}
