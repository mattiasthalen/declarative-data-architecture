CREATE TABLE IF NOT EXISTS "das__adventure_works"."customer__historized" (
    _record_hash  VARCHAR  NOT NULL PRIMARY KEY,
    _dlt_id       VARCHAR  NOT NULL,
    _dlt_load_id  VARCHAR  NOT NULL,
    _loaded_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "customer_id"  BIGINT  NOT NULL,
    "company_name"  VARCHAR,
    "modified_date"  TIMESTAMP  NOT NULL
);
