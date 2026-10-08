-- +goose Up
-- +goose StatementBegin
ALTER TABLE goal_comments ADD COLUMN author_user_id BIGINT REFERENCES users(id);
UPDATE goal_comments SET author_user_id = 2 WHERE author_user_id IS NULL;
ALTER TABLE goal_comments ALTER COLUMN author_user_id SET NOT NULL;

ALTER TABLE key_result_comments ADD COLUMN author_user_id BIGINT REFERENCES users(id);
UPDATE key_result_comments SET author_user_id = 2 WHERE author_user_id IS NULL;
ALTER TABLE key_result_comments ALTER COLUMN author_user_id SET NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE goal_comments DROP COLUMN IF EXISTS author_user_id;
ALTER TABLE key_result_comments DROP COLUMN IF EXISTS author_user_id;

-- +goose StatementEnd
