-- Тип уведомления «заявка на доступ к пространству»: получают администраторы
-- пространства, по умолчанию выключен (включается в настройках профиля).
ALTER TABLE notifications DROP CONSTRAINT notifications_type;
ALTER TABLE notifications ADD CONSTRAINT notifications_type CHECK (
    type IN ('goal_comment','my_comment_resolved','goal_changed','kr_progress','access_requested'));

ALTER TABLE notification_preferences DROP CONSTRAINT notification_preferences_type;
ALTER TABLE notification_preferences ADD CONSTRAINT notification_preferences_type CHECK (
    type IN ('goal_comment','my_comment_resolved','goal_changed','kr_progress','access_requested'));
