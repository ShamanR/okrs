INSERT INTO teams (name) VALUES ('migration-fixture');
INSERT INTO periods (name,start_date,end_date) VALUES ('Fixture','2024-01-01','2024-03-31');
INSERT INTO goals (team_id,period_id,title,priority,weight,work_type,focus_type,sort_order)
VALUES (1,1,'Fixture goal','P1',100,'Delivery','STABILITY',1);
INSERT INTO key_results (goal_id,title,weight,kind,sort_order)
VALUES (1,'linear',30,'LINEAR',1),(1,'percent',30,'PERCENT',2),(1,'boolean',20,'BOOLEAN',3),(1,'missing meta',20,'LINEAR',4);
INSERT INTO kr_linear_meta VALUES (1,10,5,7);
INSERT INTO kr_percent_meta VALUES (2,100,180,150);
INSERT INTO kr_percent_checkpoints (key_result_id,metric_value,kr_percent) VALUES (2,150,50),(2,180,100);
INSERT INTO kr_boolean_meta VALUES (3,true);
