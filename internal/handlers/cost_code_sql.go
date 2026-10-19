package handlers

// Displayed cost code. For cost_subject.subject_code = 'OH' the subject prefix is omitted, so OH rows
// show as G00000 / G100100 (job + group + subgroup) instead of OHG...; subject_code stays 'OH' in the
// DB and every FK / lookup keeps using ids and subject_code = 'OH'. Only the displayed code changes.
// Never use these for WHERE filters on subject/job (e.g. subject_code || job_code = ANY(job_codes)).

// costCodeSQL returns the SQL expression for the displayed 4-level cost code, given the table aliases
// of cost_subject, cost_job, cost_group and cost_subgroup in the surrounding query.
func costCodeSQL(subj, job, grp, sg string) string {
	return "(" + costCodeMainSQL(subj, job, grp) + " || " + sg + ".subgroup_code)"
}

// costCodeMainSQL is the same rule without the subgroup part (subject? + job + group).
func costCodeMainSQL(subj, job, grp string) string {
	return "(CASE WHEN " + subj + ".subject_code = 'OH' THEN '' ELSE " + subj + ".subject_code END || " +
		job + ".job_code || " + grp + ".group_code)"
}

// costCodeLegacySQL is the pre-change form (always subject-prefixed, e.g. OHG100100). Use it only to
// keep accepting old-style input in searches / reverse lookups.
func costCodeLegacySQL(subj, job, grp, sg string) string {
	return "(" + subj + ".subject_code || " + job + ".job_code || " + grp + ".group_code || " + sg + ".subgroup_code)"
}

// costCodeMatchSQL matches a text-search parameter (already a LIKE pattern, e.g. $1) against the
// displayed code AND the legacy code.
func costCodeMatchSQL(param, subj, job, grp, sg string) string {
	return "(" + costCodeSQL(subj, job, grp, sg) + " ILIKE " + param + " OR " +
		costCodeLegacySQL(subj, job, grp, sg) + " ILIKE " + param + ")"
}
