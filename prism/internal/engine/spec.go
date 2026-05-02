package engine

// Column describes one declared column in a DAS contract, after type parsing.
type Column struct {
	SourcePath string // e.g. "CustomerID" or "Address.City"
	TargetName string // e.g. "customer_id"
	SQLType    string // dialect-rendered, e.g. "BIGINT", "DECIMAL(18,4)", "VARCHAR"
	NotNull    bool   // true when contract mode is REQUIRED
}

type HistorizedTableSpec struct {
	Schema  string
	Name    string // e.g. "customer__historized"
	Columns []Column
}

type HistorizedAppendSpec struct {
	Schema      string
	Name        string // e.g. "customer__historized"
	LakeGlob    string // e.g. "/abs/path/_lake/das/adventure_works/Customer/**/*.jsonl.gz"
	Compression string // "gzip"
	Columns     []Column
}

type CurrentViewSpec struct {
	Schema          string
	Name            string   // e.g. "customer__current"
	HistorizedTable string   // e.g. "customer__historized"
	PrimaryKey      []string // target_name list
}
