-- +goose Up
-- +goose StatementBegin
-- Доставка уведомлений во внешние каналы.

-- 1. Значение канала по умолчанию для сотрудников задаёт администратор
-- пространства при настройке канала. FALSE для существующих строк намеренно:
-- после наката поведение не меняется, пока администратор не включит канал
-- сознательно — иначе выкатка сама по себе начала бы писать людям в мессенджер.
ALTER TABLE notification_channels
    ADD COLUMN default_on BOOLEAN NOT NULL DEFAULT FALSE;

-- 2. Выбор пользователя по каналам становится разреженной картой ОТКЛОНЕНИЙ от
-- значения по умолчанию: ключа нет — значит «пользователь не высказывался», и
-- применяется значение администратора.
--
-- Список включённых каналов этого выразить не мог: он не отличал «канал
-- выключен пользователем» от «канала тогда ещё не существовало». Ровно на этом
-- различии стоит подключение нового канала — он обязан доехать до всех, включая
-- тех, кто настройки уже сохранял.
ALTER TABLE notification_preferences
    ADD COLUMN channel_overrides JSONB NOT NULL DEFAULT '{}';

-- Перенос дешёвый: доставки не существовало, поэтому в channels мог фигурировать
-- ровно один канал — «в приложении». Строка есть только у того, кто настройки
-- сохранял, значит его выбор по этому каналу явный — в обе стороны.
UPDATE notification_preferences
   SET channel_overrides = jsonb_build_object('in_app', 'in_app' = ANY(channels));

ALTER TABLE notification_preferences
    DROP COLUMN channels;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
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

-- +goose StatementEnd
