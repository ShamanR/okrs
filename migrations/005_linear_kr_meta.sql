-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kr_linear_meta (
  key_result_id INTEGER PRIMARY KEY REFERENCES key_results(id) ON DELETE CASCADE,
  start_value DOUBLE PRECISION NOT NULL,
  target_value DOUBLE PRECISION NOT NULL,
  current_value DOUBLE PRECISION NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS kr_linear_meta;

-- +goose StatementEnd
