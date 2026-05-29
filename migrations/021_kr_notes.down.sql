CREATE TABLE key_result_comments (
  id             BIGSERIAL PRIMARY KEY,
  key_result_id  BIGINT NOT NULL REFERENCES key_results(id) ON DELETE CASCADE,
  text           TEXT   NOT NULL,
  author_user_id BIGINT NOT NULL REFERENCES users(id),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO key_result_comments (key_result_id, text, author_user_id, created_at)
SELECT key_result_id, text, author_user_id, updated_at
FROM key_result_notes;

DROP TABLE key_result_notes;
