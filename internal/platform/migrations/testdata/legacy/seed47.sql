INSERT INTO notifications (tenant_id,user_id,type,kind,actor_user_id,goal_id,coalesce_key,payload_json)
VALUES (1,1,'goal_changed','goal_changed',2,1,'migration-fixture','{"preserve":true}');
INSERT INTO notification_preferences (tenant_id,user_id,type,enabled) VALUES (1,1,'goal_changed',false);
