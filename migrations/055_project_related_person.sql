-- People involved in a project (besides its responsible persons in project_responsible_person).
-- Together they decide which projects a user of the Project department (departments.dept_code = 'PJ') may see.
-- Idempotent. Dev: run directly. UAT/prod: run this file manually.

CREATE TABLE IF NOT EXISTS public.project_related_person (
    project_id bigint NOT NULL REFERENCES public.project(id) ON DELETE CASCADE,
    user_id    bigint NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    created_by bigint,
    PRIMARY KEY (project_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_prelp_user_id ON public.project_related_person (user_id);
