-- How To use: tutorial videos grouped by section.
--   howto_section.menu_code = the menu whose "read" permission is required to watch that section's videos
--   (NULL = any logged-in user). Permissions are therefore managed in the normal Permission Matrix.
-- Menus: MENU_HOWTO (main "How To use") with two children:
--   MENU_HOWTO_VIDEOS  "video การใช้งาน"  -> /howto/videos  (read = open the page)
--   MENU_HOWTO_UPLOAD  "upload วิดีโอ"     -> /howto/upload  (read = open, write = add section/video,
--                                                              update = edit, delete = delete)
-- Idempotent. Dev: run directly. UAT/prod: run this file manually.

CREATE TABLE IF NOT EXISTS public.howto_section (
    id          bigserial PRIMARY KEY,
    name        varchar(200) NOT NULL,
    description text,
    menu_code   varchar(100) REFERENCES public.menus(menu_code) ON UPDATE CASCADE ON DELETE RESTRICT,
    sort_order  integer      NOT NULL DEFAULT 0,
    is_active   boolean      NOT NULL DEFAULT true,
    created_at  timestamp    NOT NULL DEFAULT now(),
    updated_at  timestamp    NOT NULL DEFAULT now(),
    created_by  bigint REFERENCES public.users(id),
    updated_by  bigint REFERENCES public.users(id)
);

CREATE TABLE IF NOT EXISTS public.howto_video (
    id            bigserial PRIMARY KEY,
    section_id    bigint       NOT NULL REFERENCES public.howto_section(id) ON DELETE CASCADE,
    title         varchar(200) NOT NULL,
    description   text,
    file_path     text         NOT NULL,   -- relative to the upload root, e.g. howto/12/<uuid>.mp4 (never served by /uploads)
    original_name text,
    mime_type     varchar(100),
    size_bytes    bigint,
    sort_order    integer      NOT NULL DEFAULT 0,
    created_at    timestamp    NOT NULL DEFAULT now(),
    created_by    bigint REFERENCES public.users(id)
);
CREATE INDEX IF NOT EXISTS idx_howto_video_section ON public.howto_video(section_id);

-- Main menu (top level, after Administration).
INSERT INTO public.menus (parent_id, menu_code, menu_name, menu_path, icon_name, sort_order, is_active)
SELECT NULL, 'MENU_HOWTO', 'How To use', '/howto', NULL,
       COALESCE((SELECT MAX(sort_order) FROM public.menus WHERE parent_id IS NULL), 0) + 1, true
WHERE NOT EXISTS (SELECT 1 FROM public.menus x WHERE x.menu_code = 'MENU_HOWTO');

INSERT INTO public.menus (parent_id, menu_code, menu_name, menu_path, icon_name, sort_order, is_active)
SELECT p.id, 'MENU_HOWTO_VIDEOS', 'video การใช้งาน', '/howto/videos', NULL, 1, true
FROM public.menus p
WHERE p.menu_code = 'MENU_HOWTO'
  AND NOT EXISTS (SELECT 1 FROM public.menus x WHERE x.menu_code = 'MENU_HOWTO_VIDEOS');

INSERT INTO public.menus (parent_id, menu_code, menu_name, menu_path, icon_name, sort_order, is_active)
SELECT p.id, 'MENU_HOWTO_UPLOAD', 'upload วิดีโอ', '/howto/upload', NULL, 2, true
FROM public.menus p
WHERE p.menu_code = 'MENU_HOWTO'
  AND NOT EXISTS (SELECT 1 FROM public.menus x WHERE x.menu_code = 'MENU_HOWTO_UPLOAD');

-- Permissions: every role may open the main menu and the viewer page (what each user sees is then filtered
-- per section by the section's own menu); only ADMIN_CENTER may manage (upload page).
INSERT INTO public.role_menu_permissions (role_id, menu_id, can_read, can_write, can_update, can_delete)
SELECT r.id, m.id, true, false, false, false
FROM public.roles r
JOIN public.menus m ON m.menu_code IN ('MENU_HOWTO', 'MENU_HOWTO_VIDEOS')
WHERE NOT EXISTS (SELECT 1 FROM public.role_menu_permissions x WHERE x.role_id = r.id AND x.menu_id = m.id);

INSERT INTO public.role_menu_permissions (role_id, menu_id, can_read, can_write, can_update, can_delete)
SELECT r.id, m.id, true, true, true, true
FROM public.roles r
JOIN public.menus m ON m.menu_code = 'MENU_HOWTO_UPLOAD'
WHERE r.role_code = 'ADMIN_CENTER'
  AND NOT EXISTS (SELECT 1 FROM public.role_menu_permissions x WHERE x.role_id = r.id AND x.menu_id = m.id);
