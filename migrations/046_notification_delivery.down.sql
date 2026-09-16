-- Возврат к списку включённых каналов. Восстанавливается только то, что этот
-- список способен выразить: «в приложении» включён, если пользователь его явно
-- не выключал. Отклонения по внешним каналам при откате теряются — выразить их
-- в списке, который не отличает «выключено» от «не высказывался», нечем.
ALTER TABLE notification_preferences
    ADD COLUMN channels TEXT[] NOT NULL DEFAULT '{in_app}';

UPDATE notification_preferences
   SET channels = CASE
           WHEN channel_overrides->>'in_app' = 'false' THEN '{}'::text[]
           ELSE '{in_app}'::text[]
       END;

ALTER TABLE notification_preferences
    DROP COLUMN channel_overrides;

ALTER TABLE notification_channels
    DROP COLUMN default_on;
