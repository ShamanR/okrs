CREATE TABLE key_result_notes (
  key_result_id  BIGINT PRIMARY KEY REFERENCES key_results(id) ON DELETE CASCADE,
  text           TEXT        NOT NULL,
  author_user_id BIGINT      NOT NULL REFERENCES users(id),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO key_result_notes (key_result_id, text, author_user_id, updated_at)
SELECT DISTINCT ON (key_result_id)
  key_result_id, text, author_user_id, created_at
FROM key_result_comments
ORDER BY key_result_id, created_at DESC;

DROP TABLE key_result_comments;
