--
-- PostgreSQL database dump
--

\restrict uxAPvauyJOrFS1mrhxYeNDGBY6Jf0sSpXO5eKrao5tnRPjZp0fYHkfQTgjcFWoQ

-- Dumped from database version 15.18 (Debian 15.18-1.pgdg13+1)
-- Dumped by pg_dump version 15.18 (Debian 15.18-1.pgdg13+1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pgcrypto IS 'cryptographic functions';


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: auth_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.auth_sessions (
    id text NOT NULL,
    user_id bigint NOT NULL,
    provider text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    user_agent text,
    ip text
);


--
-- Name: goal_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.goal_comments (
    id integer NOT NULL,
    goal_id integer NOT NULL,
    text text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    author_user_id bigint NOT NULL
);


--
-- Name: goal_comments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.goal_comments_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: goal_comments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.goal_comments_id_seq OWNED BY public.goal_comments.id;


--
-- Name: goal_shares; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.goal_shares (
    goal_id integer NOT NULL,
    team_id integer NOT NULL,
    weight integer NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT goal_shares_weight_check CHECK (((weight >= 0) AND (weight <= 100)))
);


--
-- Name: goals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.goals (
    id integer NOT NULL,
    team_id integer NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    priority text NOT NULL,
    weight integer NOT NULL,
    work_type text NOT NULL,
    focus_type text NOT NULL,
    owner_text text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    period_id integer NOT NULL,
    owner_udids uuid[],
    CONSTRAINT goals_weight_check CHECK (((weight >= 0) AND (weight <= 100)))
);


--
-- Name: goals_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.goals_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: goals_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.goals_id_seq OWNED BY public.goals.id;


--
-- Name: key_result_notes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.key_result_notes (
    key_result_id bigint NOT NULL,
    text text NOT NULL,
    author_user_id bigint NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: key_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.key_results (
    id integer NOT NULL,
    goal_id integer NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    weight integer NOT NULL,
    kind text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    progress_updated_at timestamp with time zone,
    CONSTRAINT key_results_weight_check CHECK (((weight >= 0) AND (weight <= 100)))
);


--
-- Name: key_results_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.key_results_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: key_results_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.key_results_id_seq OWNED BY public.key_results.id;


--
-- Name: kr_boolean_meta; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kr_boolean_meta (
    key_result_id integer NOT NULL,
    is_done boolean DEFAULT false NOT NULL
);


--
-- Name: kr_linear_meta; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kr_linear_meta (
    key_result_id integer NOT NULL,
    start_value double precision NOT NULL,
    target_value double precision NOT NULL,
    current_value double precision NOT NULL
);


--
-- Name: kr_percent_checkpoints; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kr_percent_checkpoints (
    id integer NOT NULL,
    key_result_id integer NOT NULL,
    metric_value double precision NOT NULL,
    kr_percent integer NOT NULL,
    CONSTRAINT kr_percent_checkpoints_kr_percent_check CHECK (((kr_percent >= 0) AND (kr_percent <= 100)))
);


--
-- Name: kr_percent_checkpoints_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.kr_percent_checkpoints_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: kr_percent_checkpoints_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.kr_percent_checkpoints_id_seq OWNED BY public.kr_percent_checkpoints.id;


--
-- Name: kr_percent_meta; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kr_percent_meta (
    key_result_id integer NOT NULL,
    start_value double precision NOT NULL,
    target_value double precision NOT NULL,
    current_value double precision NOT NULL
);


--
-- Name: kr_project_stages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kr_project_stages (
    id integer NOT NULL,
    key_result_id integer NOT NULL,
    title text NOT NULL,
    weight integer NOT NULL,
    is_done boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    CONSTRAINT kr_project_stages_weight_check CHECK (((weight >= 0) AND (weight <= 100)))
);


--
-- Name: kr_project_stages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.kr_project_stages_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: kr_project_stages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.kr_project_stages_id_seq OWNED BY public.kr_project_stages.id;


--
-- Name: periods; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.periods (
    id integer NOT NULL,
    name text NOT NULL,
    start_date date NOT NULL,
    end_date date NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: periods_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.periods_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: periods_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.periods_id_seq OWNED BY public.periods.id;


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version bigint NOT NULL,
    dirty boolean NOT NULL
);


--
-- Name: system_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.system_settings (
    key text NOT NULL,
    value_json jsonb DEFAULT 'null'::jsonb NOT NULL
);


--
-- Name: team_period_statuses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.team_period_statuses (
    team_id integer NOT NULL,
    period_id integer NOT NULL,
    status text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: teams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.teams (
    id integer NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    team_type text DEFAULT 'team'::text NOT NULL,
    parent_id integer,
    lead text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    deleted_at timestamp with time zone,
    lead_udid uuid,
    CONSTRAINT teams_team_type_check CHECK ((team_type = ANY (ARRAY['cluster'::text, 'unit'::text, 'team'::text])))
);


--
-- Name: teams_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.teams_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: teams_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.teams_id_seq OWNED BY public.teams.id;


--
-- Name: user_hierarchy_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_hierarchy_grants (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    team_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by_user_id bigint NOT NULL
);


--
-- Name: user_hierarchy_grants_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_hierarchy_grants_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_hierarchy_grants_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_hierarchy_grants_id_seq OWNED BY public.user_hierarchy_grants.id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    provider_subject_key text NOT NULL,
    provider text NOT NULL,
    subject text NOT NULL,
    display_name text NOT NULL,
    avatar_url text DEFAULT ''::text NOT NULL,
    email text,
    attributes_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    is_admin boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    last_login_at timestamp with time zone DEFAULT now() NOT NULL,
    udid uuid DEFAULT gen_random_uuid() NOT NULL
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: goal_comments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goal_comments ALTER COLUMN id SET DEFAULT nextval('public.goal_comments_id_seq'::regclass);


--
-- Name: goals id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goals ALTER COLUMN id SET DEFAULT nextval('public.goals_id_seq'::regclass);


--
-- Name: key_results id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.key_results ALTER COLUMN id SET DEFAULT nextval('public.key_results_id_seq'::regclass);


--
-- Name: kr_percent_checkpoints id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_percent_checkpoints ALTER COLUMN id SET DEFAULT nextval('public.kr_percent_checkpoints_id_seq'::regclass);


--
-- Name: kr_project_stages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_project_stages ALTER COLUMN id SET DEFAULT nextval('public.kr_project_stages_id_seq'::regclass);


--
-- Name: periods id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.periods ALTER COLUMN id SET DEFAULT nextval('public.periods_id_seq'::regclass);


--
-- Name: teams id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams ALTER COLUMN id SET DEFAULT nextval('public.teams_id_seq'::regclass);


--
-- Name: user_hierarchy_grants id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_hierarchy_grants ALTER COLUMN id SET DEFAULT nextval('public.user_hierarchy_grants_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Data for Name: auth_sessions; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.auth_sessions (id, user_id, provider, created_at, expires_at, last_seen_at, user_agent, ip) FROM stdin;
\.


--
-- Data for Name: goal_comments; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.goal_comments (id, goal_id, text, created_at, author_user_id) FROM stdin;
\.


--
-- Data for Name: goal_shares; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.goal_shares (goal_id, team_id, weight, sort_order, created_at, updated_at) FROM stdin;
\.


--
-- Data for Name: goals; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.goals (id, team_id, title, description, priority, weight, work_type, focus_type, owner_text, created_at, updated_at, sort_order, period_id, owner_udids) FROM stdin;
1	1	Fixture goal		P1	100	Delivery	STABILITY		2026-10-08 13:10:23.581774+00	2026-10-08 13:10:23.581774+00	1	1	\N
\.


--
-- Data for Name: key_result_notes; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.key_result_notes (key_result_id, text, author_user_id, updated_at) FROM stdin;
\.


--
-- Data for Name: key_results; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.key_results (id, goal_id, title, description, weight, kind, created_at, updated_at, sort_order, progress_updated_at) FROM stdin;
1	1	linear		30	LINEAR	2026-10-08 13:10:23.584589+00	2026-10-08 13:10:23.584589+00	1	\N
2	1	percent		30	PERCENT	2026-10-08 13:10:23.584589+00	2026-10-08 13:10:23.584589+00	2	\N
3	1	boolean		20	BOOLEAN	2026-10-08 13:10:23.584589+00	2026-10-08 13:10:23.584589+00	3	\N
4	1	missing meta		20	LINEAR	2026-10-08 13:10:23.584589+00	2026-10-08 13:10:23.584589+00	4	\N
\.


--
-- Data for Name: kr_boolean_meta; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.kr_boolean_meta (key_result_id, is_done) FROM stdin;
3	t
\.


--
-- Data for Name: kr_linear_meta; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.kr_linear_meta (key_result_id, start_value, target_value, current_value) FROM stdin;
1	10	5	7
\.


--
-- Data for Name: kr_percent_checkpoints; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.kr_percent_checkpoints (id, key_result_id, metric_value, kr_percent) FROM stdin;
1	2	150	50
2	2	180	100
\.


--
-- Data for Name: kr_percent_meta; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.kr_percent_meta (key_result_id, start_value, target_value, current_value) FROM stdin;
2	100	180	150
\.


--
-- Data for Name: kr_project_stages; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.kr_project_stages (id, key_result_id, title, weight, is_done, sort_order) FROM stdin;
\.


--
-- Data for Name: periods; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.periods (id, name, start_date, end_date, sort_order, created_at, updated_at) FROM stdin;
1	Fixture	2024-01-01	2024-03-31	0	2026-10-08 13:10:23.579515+00	2026-10-08 13:10:23.579515+00
\.


--
-- Data for Name: schema_migrations; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.schema_migrations (version, dirty) FROM stdin;
22	f
\.


--
-- Data for Name: system_settings; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.system_settings (key, value_json) FROM stdin;
new_user_policy	"empty"
default_hierarchy_node_id	null
\.


--
-- Data for Name: team_period_statuses; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.team_period_statuses (team_id, period_id, status, updated_at) FROM stdin;
\.


--
-- Data for Name: teams; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.teams (id, name, created_at, updated_at, team_type, parent_id, lead, description, deleted_at, lead_udid) FROM stdin;
1	migration-fixture	2026-10-08 13:10:23.57652+00	2026-10-08 13:10:23.57652+00	team	\N			\N	\N
\.


--
-- Data for Name: user_hierarchy_grants; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.user_hierarchy_grants (id, user_id, team_id, created_at, created_by_user_id) FROM stdin;
\.


--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: -
--

COPY public.users (id, provider_subject_key, provider, subject, display_name, avatar_url, email, attributes_json, is_admin, created_at, updated_at, last_login_at, udid) FROM stdin;
1	system:anonymous-local	system	anonymous-local	Anonymous		\N	{}	f	2026-10-08 13:10:22.996006+00	2026-10-08 13:10:22.996006+00	2026-10-08 13:10:22.996006+00	8d8a5c69-2849-4ef5-8251-c187e9bdd038
2	system:migration	system	migration	Migration		\N	{}	f	2026-10-08 13:10:22.996006+00	2026-10-08 13:10:22.996006+00	2026-10-08 13:10:22.996006+00	55719750-b919-4f38-85c4-b1ffd7145b00
\.


--
-- Name: goal_comments_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.goal_comments_id_seq', 1, false);


--
-- Name: goals_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.goals_id_seq', 1, true);


--
-- Name: key_results_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.key_results_id_seq', 4, true);


--
-- Name: kr_percent_checkpoints_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.kr_percent_checkpoints_id_seq', 2, true);


--
-- Name: kr_project_stages_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.kr_project_stages_id_seq', 1, false);


--
-- Name: periods_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.periods_id_seq', 1, true);


--
-- Name: teams_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.teams_id_seq', 1, true);


--
-- Name: user_hierarchy_grants_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.user_hierarchy_grants_id_seq', 1, false);


--
-- Name: users_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.users_id_seq', 100, true);


--
-- Name: auth_sessions auth_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_pkey PRIMARY KEY (id);


--
-- Name: goal_comments goal_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goal_comments
    ADD CONSTRAINT goal_comments_pkey PRIMARY KEY (id);


--
-- Name: goal_shares goal_shares_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goal_shares
    ADD CONSTRAINT goal_shares_pkey PRIMARY KEY (goal_id, team_id);


--
-- Name: goals goals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goals
    ADD CONSTRAINT goals_pkey PRIMARY KEY (id);


--
-- Name: key_result_notes key_result_notes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.key_result_notes
    ADD CONSTRAINT key_result_notes_pkey PRIMARY KEY (key_result_id);


--
-- Name: key_results key_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.key_results
    ADD CONSTRAINT key_results_pkey PRIMARY KEY (id);


--
-- Name: kr_boolean_meta kr_boolean_meta_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_boolean_meta
    ADD CONSTRAINT kr_boolean_meta_pkey PRIMARY KEY (key_result_id);


--
-- Name: kr_linear_meta kr_linear_meta_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_linear_meta
    ADD CONSTRAINT kr_linear_meta_pkey PRIMARY KEY (key_result_id);


--
-- Name: kr_percent_checkpoints kr_percent_checkpoints_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_percent_checkpoints
    ADD CONSTRAINT kr_percent_checkpoints_pkey PRIMARY KEY (id);


--
-- Name: kr_percent_meta kr_percent_meta_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_percent_meta
    ADD CONSTRAINT kr_percent_meta_pkey PRIMARY KEY (key_result_id);


--
-- Name: kr_project_stages kr_project_stages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_project_stages
    ADD CONSTRAINT kr_project_stages_pkey PRIMARY KEY (id);


--
-- Name: periods periods_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.periods
    ADD CONSTRAINT periods_name_key UNIQUE (name);


--
-- Name: periods periods_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.periods
    ADD CONSTRAINT periods_pkey PRIMARY KEY (id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: system_settings system_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_settings
    ADD CONSTRAINT system_settings_pkey PRIMARY KEY (key);


--
-- Name: team_period_statuses team_period_statuses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_period_statuses
    ADD CONSTRAINT team_period_statuses_pkey PRIMARY KEY (team_id, period_id);


--
-- Name: teams teams_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_name_key UNIQUE (name);


--
-- Name: teams teams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_pkey PRIMARY KEY (id);


--
-- Name: user_hierarchy_grants user_hierarchy_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_hierarchy_grants
    ADD CONSTRAINT user_hierarchy_grants_pkey PRIMARY KEY (id);


--
-- Name: user_hierarchy_grants user_hierarchy_grants_user_id_team_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_hierarchy_grants
    ADD CONSTRAINT user_hierarchy_grants_user_id_team_id_key UNIQUE (user_id, team_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_provider_subject_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_provider_subject_key_key UNIQUE (provider_subject_key);


--
-- Name: auth_sessions_expires_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX auth_sessions_expires_at_idx ON public.auth_sessions USING btree (expires_at);


--
-- Name: auth_sessions_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX auth_sessions_user_id_idx ON public.auth_sessions USING btree (user_id);


--
-- Name: goal_shares_team_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX goal_shares_team_idx ON public.goal_shares USING btree (team_id);


--
-- Name: goals_team_period_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX goals_team_period_idx ON public.goals USING btree (team_id, period_id);


--
-- Name: goals_team_period_sort_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX goals_team_period_sort_idx ON public.goals USING btree (team_id, period_id, sort_order);


--
-- Name: key_results_goal_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX key_results_goal_idx ON public.key_results USING btree (goal_id);


--
-- Name: key_results_goal_sort_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX key_results_goal_sort_idx ON public.key_results USING btree (goal_id, sort_order);


--
-- Name: key_results_progress_updated_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX key_results_progress_updated_at_idx ON public.key_results USING btree (progress_updated_at);


--
-- Name: teams_deleted_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX teams_deleted_at_idx ON public.teams USING btree (deleted_at);


--
-- Name: teams_lead_udid_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX teams_lead_udid_idx ON public.teams USING btree (lead_udid);


--
-- Name: teams_parent_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX teams_parent_id_idx ON public.teams USING btree (parent_id);


--
-- Name: user_hierarchy_grants_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX user_hierarchy_grants_user_id_idx ON public.user_hierarchy_grants USING btree (user_id);


--
-- Name: users_provider_subject_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX users_provider_subject_idx ON public.users USING btree (provider, subject);


--
-- Name: users_udid_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX users_udid_idx ON public.users USING btree (udid);


--
-- Name: auth_sessions auth_sessions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: goal_comments goal_comments_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goal_comments
    ADD CONSTRAINT goal_comments_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id);


--
-- Name: goal_comments goal_comments_goal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goal_comments
    ADD CONSTRAINT goal_comments_goal_id_fkey FOREIGN KEY (goal_id) REFERENCES public.goals(id) ON DELETE CASCADE;


--
-- Name: goal_shares goal_shares_goal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goal_shares
    ADD CONSTRAINT goal_shares_goal_id_fkey FOREIGN KEY (goal_id) REFERENCES public.goals(id) ON DELETE CASCADE;


--
-- Name: goal_shares goal_shares_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goal_shares
    ADD CONSTRAINT goal_shares_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: goals goals_period_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goals
    ADD CONSTRAINT goals_period_fk FOREIGN KEY (period_id) REFERENCES public.periods(id) ON DELETE CASCADE;


--
-- Name: goals goals_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goals
    ADD CONSTRAINT goals_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: key_result_notes key_result_notes_author_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.key_result_notes
    ADD CONSTRAINT key_result_notes_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id);


--
-- Name: key_result_notes key_result_notes_key_result_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.key_result_notes
    ADD CONSTRAINT key_result_notes_key_result_id_fkey FOREIGN KEY (key_result_id) REFERENCES public.key_results(id) ON DELETE CASCADE;


--
-- Name: key_results key_results_goal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.key_results
    ADD CONSTRAINT key_results_goal_id_fkey FOREIGN KEY (goal_id) REFERENCES public.goals(id) ON DELETE CASCADE;


--
-- Name: kr_boolean_meta kr_boolean_meta_key_result_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_boolean_meta
    ADD CONSTRAINT kr_boolean_meta_key_result_id_fkey FOREIGN KEY (key_result_id) REFERENCES public.key_results(id) ON DELETE CASCADE;


--
-- Name: kr_linear_meta kr_linear_meta_key_result_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_linear_meta
    ADD CONSTRAINT kr_linear_meta_key_result_id_fkey FOREIGN KEY (key_result_id) REFERENCES public.key_results(id) ON DELETE CASCADE;


--
-- Name: kr_percent_checkpoints kr_percent_checkpoints_key_result_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_percent_checkpoints
    ADD CONSTRAINT kr_percent_checkpoints_key_result_id_fkey FOREIGN KEY (key_result_id) REFERENCES public.key_results(id) ON DELETE CASCADE;


--
-- Name: kr_percent_meta kr_percent_meta_key_result_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_percent_meta
    ADD CONSTRAINT kr_percent_meta_key_result_id_fkey FOREIGN KEY (key_result_id) REFERENCES public.key_results(id) ON DELETE CASCADE;


--
-- Name: kr_project_stages kr_project_stages_key_result_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kr_project_stages
    ADD CONSTRAINT kr_project_stages_key_result_id_fkey FOREIGN KEY (key_result_id) REFERENCES public.key_results(id) ON DELETE CASCADE;


--
-- Name: team_period_statuses team_period_statuses_period_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_period_statuses
    ADD CONSTRAINT team_period_statuses_period_id_fkey FOREIGN KEY (period_id) REFERENCES public.periods(id) ON DELETE CASCADE;


--
-- Name: team_period_statuses team_period_statuses_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_period_statuses
    ADD CONSTRAINT team_period_statuses_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: teams teams_lead_udid_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_lead_udid_fkey FOREIGN KEY (lead_udid) REFERENCES public.users(udid) ON DELETE SET NULL;


--
-- Name: teams teams_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.teams(id) ON DELETE SET NULL;


--
-- Name: user_hierarchy_grants user_hierarchy_grants_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_hierarchy_grants
    ADD CONSTRAINT user_hierarchy_grants_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id);


--
-- Name: user_hierarchy_grants user_hierarchy_grants_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_hierarchy_grants
    ADD CONSTRAINT user_hierarchy_grants_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: user_hierarchy_grants user_hierarchy_grants_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_hierarchy_grants
    ADD CONSTRAINT user_hierarchy_grants_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict uxAPvauyJOrFS1mrhxYeNDGBY6Jf0sSpXO5eKrao5tnRPjZp0fYHkfQTgjcFWoQ

