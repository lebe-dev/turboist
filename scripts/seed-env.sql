-- scripts/seed-env.sql
--
-- Populates a freshly-migrated turboist SQLite database with realistic demo
-- content: 4 contexts, 10 labels, 9 projects (3-3-3 across the Troiki slots,
-- mixed generic/software), project sections, ~40 tasks with subtasks, an Inbox
-- backlog, and a recurring task. Enables the Troiki system in user settings.
--
-- Idempotent: every run wipes content tables and re-inserts. The `users` and
-- `app_settings` rows are preserved; user.settings.troikiEnabled is forced to
-- true and Troiki capacities are reset to the initial-fill state.

PRAGMA foreign_keys = ON;

BEGIN TRANSACTION;

-- ============================================================================
-- Wipe content
-- ============================================================================
DELETE FROM task_labels;
DELETE FROM project_labels;
DELETE FROM tasks;
DELETE FROM project_sections;
DELETE FROM projects;
DELETE FROM labels;
DELETE FROM contexts;
DELETE FROM sqlite_sequence
 WHERE name IN ('tasks','projects','project_sections','labels','contexts');

-- Troiki state: initial-fill (Started=false), Important cap=3, Medium/Rest
-- start at 3 via the initial-fill rule.
UPDATE users
   SET troiki_started        = 0,
       troiki_medium_capacity = 0,
       troiki_rest_capacity   = 0
 WHERE id = 1;

-- Enable Troiki in user settings (merge into existing JSON).
UPDATE users
   SET settings = json_set(
           CASE WHEN settings = '' THEN '{}' ELSE settings END,
           '$.troikiEnabled', json('true'))
 WHERE id = 1;

-- ============================================================================
-- Contexts (4)
-- ============================================================================
INSERT INTO contexts (id, name, color, is_favourite, created_at, updated_at) VALUES
  (1, 'Work',     'blue',   1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (2, 'Personal', 'green',  1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (3, 'Home',     'orange', 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (4, 'Learning', 'purple', 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ============================================================================
-- Labels (10)
-- ============================================================================
INSERT INTO labels (id, name, color, is_favourite, created_at, updated_at) VALUES
  (1,  'call',      'pink',   0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (2,  'email',     'blue',   0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (3,  'code',      'teal',   1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (4,  'review',    'yellow', 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (5,  'meeting',   'orange', 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (6,  'design',    'purple', 0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (7,  'research',  'grey',   0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (8,  'deep-work', 'brown',  1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (9,  'bug',       'red',    0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (10, 'errand',    'green',  0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ============================================================================
-- Projects (9, all assigned to a Troiki slot)
--   Important slot (priority=high tasks):     ids 1, 2, 3
--   Medium    slot (priority=medium tasks):   ids 4, 5, 6
--   Rest      slot (priority=low tasks):      ids 7, 8, 9
-- ============================================================================
INSERT INTO projects
  (id, context_id, title, description, color, status,
   is_pinned, pinned_at, project_type, troiki_category, created_at, updated_at)
VALUES
  -- Important
  (1, 1, 'Q4 Product Launch',
        'Deliver the MVP and ship to early customers.',
        'red',    'open',
        1, strftime('%Y-%m-%dT%H:%M:%fZ','now'),
        'software', 'important',
        strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (2, 1, 'Annual Performance Review',
        'Self-review, 360 feedback, goals for next year.',
        'orange', 'open',
        0, NULL,
        'generic', 'important',
        strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (3, 2, 'Tax Filing 2026',
        'Gather documents, file federal and state returns.',
        'yellow', 'open',
        1, strftime('%Y-%m-%dT%H:%M:%fZ','now'),
        'generic', 'important',
        strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  -- Medium
  (4, 1, 'API Documentation',
        'Polish OpenAPI spec and publish client guides.',
        'teal',   'open',
        0, NULL,
        'software', 'medium',
        strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (5, 2, 'Apartment Search',
        'Find a new place to live by end of summer.',
        'green',  'open',
        0, NULL,
        'generic',  'medium',
        strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (6, 4, 'React Mastery Course',
        'Finish the advanced React patterns course.',
        'blue',   'open',
        0, NULL,
        'generic',  'medium',
        strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  -- Rest
  (7, 2, 'Side Project: Blog Engine',
        'Build a tiny static-site generator with Markdown and RSS.',
        'purple', 'open',
        0, NULL,
        'software', 'rest',
        strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (8, 4, 'Read Designing Data-Intensive Applications',
        'One chapter per week, take notes.',
        'brown',  'open',
        0, NULL,
        'generic',  'rest',
        strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (9, 3, 'Garden Renovation',
        'Plan the layout, buy plants, install a drip line.',
        'pink',   'open',
        0, NULL,
        'generic',  'rest',
        strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ============================================================================
-- Project sections (only for software / multi-stage projects)
-- ============================================================================
INSERT INTO project_sections (id, project_id, title, position, created_at, updated_at) VALUES
  -- Q4 Product Launch (1, software)
  (1, 1, 'Backlog',     0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (2, 1, 'In Progress', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (3, 1, 'Review',      2, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  -- API Documentation (4, software)
  (4, 4, 'Endpoints',   0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (5, 4, 'Guides',      1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  -- Blog Engine (7, software)
  (6, 7, 'Core',        0, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (7, 7, 'Polish',      1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ============================================================================
-- Tasks
-- ----------------------------------------------------------------------------
-- Conventions:
--   * Inbox tasks set inbox_id=1, context_id=NULL, no project/section/parent.
--   * Project tasks set context_id to the project's context_id; priority is
--     pinned by the project's Troiki category (high/medium/low).
--   * Subtasks set parent_id, inherit project_id (and section_id where useful).
-- Explicit IDs are used so we can reference parents from subtasks below.
-- ============================================================================

-- ----- Project 1: Q4 Product Launch (Important / high) ----------------------
INSERT INTO tasks
  (id, title, description, context_id, project_id, section_id, parent_id,
   priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
   day_part, plan_state, is_pinned, pinned_at, recurrence_rule,
   created_at, updated_at)
VALUES
  (1, 'Define MVP scope', 'List the smallest set of features we can ship.',
       1, 1, 2, NULL,
       'high', 'open',
       strftime('%Y-%m-%dT09:00:00.000Z','now','+1 day'), 1, NULL, 0,
       'morning', 'week', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (2, 'Draft user stories', '', 1, 1, 2, 1,
       'high', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (3, 'Write API contract', '', 1, 1, 2, 1,
       'high', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),

  (4, 'Implement auth flow', 'JWT access + rotating refresh.',
       1, 1, 2, NULL,
       'high', 'open',
       strftime('%Y-%m-%dT%H:%M:%fZ','now','+3 days'), 0, NULL, 0,
       'afternoon', 'week', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (5, 'Login endpoint', '', 1, 1, 2, 4,
       'high', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (6, 'Refresh token rotation', '', 1, 1, 2, 4,
       'high', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),

  (7, 'Set up CI pipeline', '', 1, 1, 1, NULL,
       'high', 'open', NULL, 0, NULL, 0,
       'none', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (8, 'Ship MVP to staging', 'Smoke test happy path and rollback drill.',
       1, 1, 3, NULL,
       'high', 'open',
       strftime('%Y-%m-%dT17:00:00.000Z','now','+5 days'), 1,
       strftime('%Y-%m-%dT%H:%M:%fZ','now','+10 days'), 0,
       'evening', 'week', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ----- Project 2: Annual Performance Review (Important / high) --------------
INSERT INTO tasks
  (id, title, description, context_id, project_id, section_id, parent_id,
   priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
   day_part, plan_state, is_pinned, pinned_at, recurrence_rule,
   created_at, updated_at)
VALUES
  (9, 'Write self-review', '', 1, 2, NULL, NULL,
       'high', 'open',
       strftime('%Y-%m-%dT%H:%M:%fZ','now','+2 days'), 0, NULL, 0,
       'morning', 'week', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (10, 'List shipped projects', '', 1, 2, NULL, 9,
       'high', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (11, 'Reflect on growth areas', '', 1, 2, NULL, 9,
       'high', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),

  (12, 'Schedule 360 feedback round', '', 1, 2, NULL, NULL,
       'high', 'open', NULL, 0, NULL, 0,
       'none', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (13, 'Draft next year goals', '', 1, 2, NULL, NULL,
       'high', 'open',
       strftime('%Y-%m-%dT%H:%M:%fZ','now','+7 days'), 0, NULL, 0,
       'afternoon', 'week', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ----- Project 3: Tax Filing 2026 (Important / high) ------------------------
INSERT INTO tasks
  (id, title, description, context_id, project_id, section_id, parent_id,
   priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
   day_part, plan_state, is_pinned, pinned_at, recurrence_rule,
   created_at, updated_at)
VALUES
  (14, 'Collect W-2s and 1099s', '', 2, 3, NULL, NULL,
       'high', 'open',
       strftime('%Y-%m-%dT%H:%M:%fZ','now','+1 day'), 0, NULL, 0,
       'none', 'week', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (15, 'Itemize charitable deductions', '', 2, 3, NULL, NULL,
       'high', 'open', NULL, 0, NULL, 0,
       'none', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (16, 'Submit federal return', 'Use last year''s preparer.',
       2, 3, NULL, NULL,
       'high', 'open',
       strftime('%Y-%m-%dT%H:%M:%fZ','now','+14 days'), 0,
       strftime('%Y-%m-%dT23:59:00.000Z','now','+15 days'), 1,
       'none', 'week', 1, strftime('%Y-%m-%dT%H:%M:%fZ','now'), NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ----- Project 4: API Documentation (Medium / medium) -----------------------
INSERT INTO tasks
  (id, title, description, context_id, project_id, section_id, parent_id,
   priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
   day_part, plan_state, is_pinned, pinned_at, recurrence_rule,
   created_at, updated_at)
VALUES
  (17, 'Polish OpenAPI spec', '', 1, 4, 4, NULL,
       'medium', 'open', NULL, 0, NULL, 0,
       'morning', 'week', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (18, 'Add request examples', '', 1, 4, 4, 17,
       'medium', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (19, 'Add response examples', '', 1, 4, 4, 17,
       'medium', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),

  (20, 'Write quickstart guide', '', 1, 4, 5, NULL,
       'medium', 'open', NULL, 0, NULL, 0,
       'afternoon', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (21, 'Document auth scopes', '', 1, 4, 5, NULL,
       'medium', 'open', NULL, 0, NULL, 0,
       'none', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ----- Project 5: Apartment Search (Medium / medium) ------------------------
INSERT INTO tasks
  (id, title, description, context_id, project_id, section_id, parent_id,
   priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
   day_part, plan_state, is_pinned, pinned_at, recurrence_rule,
   created_at, updated_at)
VALUES
  (22, 'List must-have criteria', '', 2, 5, NULL, NULL,
       'medium', 'open', NULL, 0, NULL, 0,
       'none', 'week', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (23, 'Browse 5 listings', '', 2, 5, NULL, NULL,
       'medium', 'open',
       strftime('%Y-%m-%dT%H:%M:%fZ','now','+2 days'), 0, NULL, 0,
       'evening', 'week', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (24, 'Tour 2 places', '', 2, 5, NULL, NULL,
       'medium', 'open',
       strftime('%Y-%m-%dT%H:%M:%fZ','now','+6 days'), 0, NULL, 0,
       'afternoon', 'week', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ----- Project 6: React Mastery Course (Medium / medium) --------------------
INSERT INTO tasks
  (id, title, description, context_id, project_id, section_id, parent_id,
   priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
   day_part, plan_state, is_pinned, pinned_at, recurrence_rule,
   created_at, updated_at)
VALUES
  (25, 'Watch hooks deep-dive module', '', 4, 6, NULL, NULL,
       'medium', 'open', NULL, 0, NULL, 0,
       'evening', 'week', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (26, 'Build a memoization demo', '', 4, 6, NULL, NULL,
       'medium', 'open', NULL, 0, NULL, 0,
       'none', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ----- Project 7: Side Project: Blog Engine (Rest / low) --------------------
INSERT INTO tasks
  (id, title, description, context_id, project_id, section_id, parent_id,
   priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
   day_part, plan_state, is_pinned, pinned_at, recurrence_rule,
   created_at, updated_at)
VALUES
  (27, 'Sketch architecture', '', 2, 7, 6, NULL,
       'low', 'open', NULL, 0, NULL, 0,
       'none', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (28, 'Implement Markdown parser', '', 2, 7, 6, NULL,
       'low', 'open', NULL, 0, NULL, 0,
       'none', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (29, 'Support fenced code blocks', '', 2, 7, 6, 28,
       'low', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (30, 'Generate RSS feed', '', 2, 7, 7, NULL,
       'low', 'open', NULL, 0, NULL, 0,
       'none', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ----- Project 8: Read DDIA (Rest / low) ------------------------------------
INSERT INTO tasks
  (id, title, description, context_id, project_id, section_id, parent_id,
   priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
   day_part, plan_state, is_pinned, pinned_at, recurrence_rule,
   created_at, updated_at)
VALUES
  (31, 'Read one chapter', 'Recurring weekly reading commitment.',
       4, 8, NULL, NULL,
       'low', 'open',
       strftime('%Y-%m-%dT%H:%M:%fZ','now','+7 days'), 0, NULL, 0,
       'evening', 'none', 0, NULL,
       'FREQ=WEEKLY;BYDAY=SU',
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (32, 'Summarize chapter notes', '', 4, 8, NULL, NULL,
       'low', 'open', NULL, 0, NULL, 0,
       'none', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ----- Project 9: Garden Renovation (Rest / low) ----------------------------
INSERT INTO tasks
  (id, title, description, context_id, project_id, section_id, parent_id,
   priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
   day_part, plan_state, is_pinned, pinned_at, recurrence_rule,
   created_at, updated_at)
VALUES
  (33, 'Measure the plot', '', 3, 9, NULL, NULL,
       'low', 'open', NULL, 0, NULL, 0,
       'morning', 'week', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (34, 'Buy seedlings at the nursery', '', 3, 9, NULL, NULL,
       'low', 'open', NULL, 0, NULL, 0,
       'afternoon', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (35, 'Install drip irrigation', '', 3, 9, NULL, NULL,
       'low', 'open', NULL, 0, NULL, 0,
       'none', 'backlog', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ----- Inbox tasks ----------------------------------------------------------
INSERT INTO tasks
  (id, title, description, inbox_id, context_id, project_id, section_id, parent_id,
   priority, status, due_at, due_has_time, deadline_at, deadline_has_time,
   day_part, plan_state, is_pinned, pinned_at, recurrence_rule,
   created_at, updated_at)
VALUES
  (36, 'Reply to recruiter from Acme',         '', 1, NULL, NULL, NULL, NULL,
       'medium', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (37, 'Idea: meal-prep app with grocery sync','', 1, NULL, NULL, NULL, NULL,
       'no-priority', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (38, 'Read RFC 9421 (HTTP message signatures)','', 1, NULL, NULL, NULL, NULL,
       'low', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (39, 'Cancel unused gym membership',         '', 1, NULL, NULL, NULL, NULL,
       'low', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  (40, 'Book dentist appointment',             '', 1, NULL, NULL, NULL, NULL,
       'medium', 'open', NULL, 0, NULL, 0,
       'none', 'none', 0, NULL, NULL,
       strftime('%Y-%m-%dT%H:%M:%fZ','now'), strftime('%Y-%m-%dT%H:%M:%fZ','now'));

-- ============================================================================
-- Task labels
-- ============================================================================
INSERT INTO task_labels (task_id, label_id) VALUES
  -- Q4 Product Launch
  (1, 8), (1, 6),         -- Define MVP scope: deep-work, design
  (2, 7),                  -- Draft user stories: research
  (3, 3),                  -- Write API contract: code
  (4, 3), (4, 8),          -- Implement auth flow: code, deep-work
  (5, 3), (6, 3),          -- Login + refresh: code
  (7, 3),                  -- CI: code
  (8, 4), (8, 8),          -- Ship MVP: review, deep-work
  -- Performance review
  (9, 8),                  -- Self-review: deep-work
  (12, 1), (12, 5),        -- Schedule 360: call, meeting
  -- Tax filing
  (14, 2),                 -- Collect W-2s: email
  (16, 4),                 -- Submit federal: review
  -- API docs
  (17, 3), (17, 4),        -- Polish OpenAPI: code, review
  (20, 3),                 -- Quickstart: code
  -- Apartment search
  (23, 7),                 -- Browse listings: research
  (24, 1),                 -- Tour places: call
  -- React course
  (25, 8),                 -- Hooks deep-dive: deep-work
  (26, 3),                 -- Memoization demo: code
  -- Blog engine
  (27, 6), (28, 3), (29, 3), (30, 3),
  -- DDIA
  (31, 8),                 -- Read chapter: deep-work
  -- Garden
  (34, 10),                -- Buy seedlings: errand
  -- Inbox
  (36, 2),                 -- Reply recruiter: email
  (39, 1),                 -- Cancel gym: call
  (40, 1);                 -- Dentist: call

-- ============================================================================
-- Project labels (lightweight tagging)
-- ============================================================================
INSERT INTO project_labels (project_id, label_id) VALUES
  (1, 3), (1, 8),          -- Q4: code, deep-work
  (4, 3),                  -- API docs: code
  (7, 3),                  -- Blog engine: code
  (6, 8),                  -- React course: deep-work
  (8, 8);                  -- DDIA: deep-work

COMMIT;

-- ============================================================================
-- Summary (printed when run via sqlite3 CLI)
-- ============================================================================
SELECT 'contexts'         AS table_name, COUNT(*) AS rows FROM contexts
UNION ALL SELECT 'labels',           COUNT(*) FROM labels
UNION ALL SELECT 'projects',         COUNT(*) FROM projects
UNION ALL SELECT 'project_sections', COUNT(*) FROM project_sections
UNION ALL SELECT 'tasks',            COUNT(*) FROM tasks
UNION ALL SELECT 'task_labels',      COUNT(*) FROM task_labels
UNION ALL SELECT 'project_labels',   COUNT(*) FROM project_labels;
