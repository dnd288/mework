export const meta = {
  name: 'author-review',
  description:
    'Multi-mode code review for team leads. Two modes: (1) REVIEW mode — given an OpenSpec change slug or a PR URL, fetch the PR diff, run parallel review agents across selected dimensions (correctness, security, quality, spec), post findings as inline PR review comments on GitHub. (2) COMMENT mode — just post a custom message from the leader as a PR review comment, skip AI entirely. In both modes the reviewee picks up the comments via /opsx:address-review. Honors dryRun (collect + report; no posting).',
  phases: [
    { title: 'Preflight', detail: 'find the PR via change slug or URL; fetch diff + files + context' },
    { title: 'Review',    detail: '[review mode] parallel agents across selected dimensions' },
    { title: 'Post',      detail: '[review mode] create a PR review with inline findings' },
    { title: 'Comment',   detail: '[comment mode] post the leader\'s custom message as a PR review' },
    { title: 'Summary',   detail: 'report results and next steps for lead and reviewee' },
  ],
}

// ---------------------------------------------------------------- args & safety
let A = typeof args === 'string' ? JSON.parse(args) : args
A = A || {}
const change = A.change                 // optional: OpenSpec change slug
const pr = A.pr                         // optional: explicit PR URL/number (overrides change)
const dryRun = !!A.dryRun
const base = A.base || 'main'
const dimensions = A.dimensions || ['correctness', 'security', 'quality', 'spec']
const customPrompt = A.prompt || ''     // leader's custom instruction, injected into every dimension
const comment = A.comment || ''         // leader's custom message — comment mode (skips AI review)
const commentFile = A.commentFile || '' // optional: file path for inline comment
const commentLine = A.commentLine || 0  // optional: line number for inline comment
const reserve = A.reserveTokens || 40000

if (!change && !pr) {
  throw new Error('author-review requires either { change } (OpenSpec slug) or { pr } (PR URL/number) — or both.')
}
if (change && !/^[a-z][a-z0-9-]*$/.test(change)) {
  throw new Error('Unsafe change name (must start with a letter, kebab-case): ' + change)
}
if (comment && dryRun) {
  throw new Error('--dry-run is not supported with --comment (the comment has nothing to dry-run; post it or skip it).')
}

const isCommentMode = !!comment
const branch = change ? `feat/${change}` : null

// ---------------------------------------------------------------- Phase 1: Preflight (same for both modes)
phase('Preflight')
const PRE = {
  type: 'object', additionalProperties: false,
  required: ['ok', 'reason', 'prUrl', 'prNumber', 'owner', 'repo', 'headSha', 'files', 'baseRef'],
  properties: {
    ok: { type: 'boolean' }, reason: { type: 'string' },
    prUrl: { type: ['string', 'null'] }, prNumber: { type: ['integer', 'null'] },
    owner: { type: 'string' }, repo: { type: 'string' },
    headSha: { type: ['string', 'null'] }, baseRef: { type: 'string' },
    files: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        required: ['path', 'additions', 'deletions', 'status'],
        properties: { path: { type: 'string' }, additions: { type: 'integer' }, deletions: { type: 'integer' }, status: { type: 'string' } },
      },
    },
  },
}
const pre = await agent(
  [
    `Preflight for author-review. Use Bash (gh, git, node). Steps:`,
    `1. TOOLS: command -v gh git node; gh auth status → ok=false+reason+STOP if missing.`,
    branch
      ? `2. Find the open PR for branch "${branch}": gh pr view "${branch}" --json number,url,state,headRefName,baseRefName,headRefOid. If none OPEN → ok=false+reason+STOP.`
      : `2. Parse PR# from "${pr}" (URL or number). Use gh pr view <number> --json number,url,state,headRefName,baseRefName,headRefOid. Capture headRefName as branch.`,
    `3. Parse owner/repo from gh repo view --json name,owner.`,
    `4. Fetch the changed files: gh pr diff ${pr ? `"${pr}"` : `"${branch}"`} --name-only; also gh pr view --json files to get additions/deletions/status per file. Capture headSha from headRefOid.`,
    `5. Fetch the base ref name from the PR metadata (baseRefName).`,
    `Return owner, repo, prUrl, prNumber, headSha, baseRef, files[].`,
    `Set ok=true on success. Do NOT edit anything.`,
  ].join('\n'),
  { schema: PRE, label: 'preflight', phase: 'Preflight', agentType: 'general-purpose' },
)
if (!pre || !pre.ok) {
  return { stage: 'preflight', ok: false, reason: pre ? pre.reason : 'preflight agent returned null', change, pr }
}
log(isCommentMode
  ? `PR #${pre.prNumber}: comment mode — will post leader's message`
  : `PR #${pre.prNumber}: review mode — ${pre.files.length} file(s), head ${pre.headSha?.slice(0, 7)}`)

// ================================================================
//  MODE A — COMMENT MODE: just post the leader's message, skip AI
// ================================================================
if (isCommentMode) {
  phase('Comment')
  const cmtResult = await agent(
    [
      `Post a custom review comment from the team lead on PR #${pre.prNumber} in ${pre.owner}/${pre.repo}.`,
      `Head SHA: ${pre.headSha}.`,
      ``,
      `The leader's message is:\n"""\n${comment}\n"""`,
      commentFile ? `Make it an INLINE comment on file "${commentFile}"${commentLine ? ` at line ${commentLine}` : ''} (side: RIGHT).` : 'Post it as a PR review summary comment (not inline).',
      ``,
      commentFile
        ? [
            `Build a comments JSON and post a PR review with one inline comment:`,
            `cat > /tmp/ar-cmt-comments.json << 'JSONEOF'`,
            JSON.stringify([{ path: commentFile, line: commentLine || 1, side: 'RIGHT', body: comment }]),
            `JSONEOF`,
            ``,
            `gh api -X POST repos/${pre.owner}/${pre.repo}/pulls/${pre.prNumber}/reviews \\`,
            `  -f event="COMMENT" \\`,
            `  -f body="Author review comment from team lead" \\`,
            `  -f commit_id="${pre.headSha}" \\`,
            `  --field-json "comments=$(cat /tmp/ar-cmt-comments.json)"`,
          ].join('\n')
        : `gh api repos/${pre.owner}/${pre.repo}/pulls/${pre.prNumber}/reviews -f event="COMMENT" -f body="${comment.replace(/"/g, '\\"').replace(/\n/g, '\\n')}"`,
      ``,
      `Verify: gh api repos/${pre.owner}/${pre.repo}/pulls/${pre.prNumber}/reviews --jq 'last | {state, id}'`,
      ``,
      `Return { posted: true, reviewId: <id> } on success; { posted: false, error: "<message>" } on failure.`,
    ].join('\n'),
    {
      label: 'post-comment',
      phase: 'Comment',
      schema: {
        type: 'object', additionalProperties: false,
        required: ['posted'],
        properties: { posted: { type: 'boolean' }, reviewId: { type: ['integer', 'null'] }, error: { type: 'string' } },
      },
      agentType: 'general-purpose',
    },
  )

  phase('Summary')
  return {
    stage: 'done',
    ok: !!(cmtResult && cmtResult.posted),
    mode: 'comment',
    prUrl: pre.prUrl,
    prNumber: pre.prNumber,
    posted: !!(cmtResult && cmtResult.posted),
    reviewId: cmtResult ? cmtResult.reviewId : null,
    comment,
    nextStep: cmtResult && cmtResult.posted
      ? `Posted leader's comment on PR #${pre.prNumber} (review ID: ${cmtResult.reviewId || '?'}).\n`
        + `**Reviewee**: Run /opsx:address-review ${change || '<change>'} to pick up the comment and respond.`
      : `Failed to post comment: ${(cmtResult && cmtResult.error) || 'unknown'}. Post manually on ${pre.prUrl}.`,
  }
}

// ================================================================
//  MODE B — REVIEW MODE: AI review across selected dimensions
// ================================================================

// ---------------------------------------------------------------- Phase 2: Review
phase('Review')
if (budget && budget.total && budget.remaining() < reserve) {
  return { stage: 'review', ok: false, reason: 'budget reserve reached before review', prUrl: pre.prUrl }
}

const DIMENSIONS = {
  correctness: {
    label: 'Correctness & Bugs',
    prompt: [
      `Review the PR diff for CORRECTNESS issues: logic bugs, off-by-one, race conditions,`,
      `unhandled errors, type mismatches, incorrect assumptions, edge cases not handled.`,
      `Be specific — cite the exact file, line number (from the diff / file), and explain why it's wrong.`,
      `If the code is correct, return an empty findings array. Do NOT report style nits.`,
    ].join(' '),
  },
  security: {
    label: 'Security & Hardening',
    prompt: [
      `Review the PR diff for SECURITY issues: injection surfaces (XSS, SQL, command),`,
      `PII/data leakage, missing input validation, unsafe deserialization, missing auth checks,`,
      `hardcoded secrets, overly permissive defaults. Be specific — cite file, line, and risk.`,
      `If secure, return an empty findings array.`,
    ].join(' '),
  },
  quality: {
    label: 'Quality & Design',
    prompt: [
      `Review the PR diff for QUALITY issues: unnecessary complexity, poor naming,`,
      `code duplication, violated DRY, missing abstractions, overly coupled modules,`,
      `inefficient algorithms, missing tests for new functionality.`,
      `Be specific — cite file and line. Prefer actionable, high-signal findings over nits.`,
    ].join(' '),
  },
  spec: {
    label: 'Spec Compliance',
    prompt: [
      `Review the PR diff for SPEC COMPLIANCE.`,
      branch
        ? `This is for OpenSpec change "${change}". Check if the implementation matches the spec contract.`
        : `Check if the implementation is consistent with the PR description and any referenced specs.`,
      `Look for: missing requirements, behavior that contradicts the spec, incomplete implementations.`,
      `Be specific — cite file, line, and which requirement/scenario is violated.`,
    ].join(' '),
  },
}

// Only run the requested dimensions
const activeDimensions = dimensions.filter((d) => DIMENSIONS[d])
if (!activeDimensions.length) {
  return { stage: 'review', ok: false, reason: 'no valid dimensions requested; choose from: ' + Object.keys(DIMENSIONS).join(', ') }
}

const FINDINGS_SCHEMA = {
  type: 'object', additionalProperties: false,
  required: ['findings'],
  properties: {
    findings: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        required: ['file', 'line', 'severity', 'title', 'body'],
        properties: {
          file: { type: 'string' },
          line: { type: ['integer', 'null'] },
          severity: { type: 'string', enum: ['blocker', 'required', 'advisory', 'nit'] },
          title: { type: 'string', maxLength: 120 },
          body: { type: 'string', maxLength: 2000 },
        },
      },
    },
  },
}

// Fetch the diff text once, share it across reviewers
const diffText = await agent(
  `Fetch the full PR diff text for PR #${pre.prNumber}: gh pr diff "${branch || pre.prNumber}" -- ${pre.files.map((f) => f.path).join(' ')}. Return ONLY the raw diff text, no commentary. If the diff is empty, return "NO_DIFF".`,
  { label: 'fetch-diff', phase: 'Review' },
)

log(`reviewing ${activeDimensions.length} dimension(s): ${activeDimensions.join(', ')}` + (customPrompt ? ` with custom prompt` : ''))

const reviewResults = await parallel(
  activeDimensions.map((dim) => () =>
    agent(
      [
        `You are a code reviewer specializing in "${DIMENSIONS[dim].label}."`,
        `Review the following PR diff against the base (${pre.baseRef}) and identify findings.`,
        `PR #${pre.prNumber} in ${pre.owner}/${pre.repo} — branch ${branch || '(direct PR)'}.`,
        ``,
        DIMENSIONS[dim].prompt,
        customPrompt ? `**Leader's additional instruction**: ${customPrompt}` : '',
        ``,
        `Diff:\n${diffText || 'NO_DIFF'}`,
        ``,
        `Return a findings array. Each finding MUST include the exact file path (as it appears in the diff)`,
        `and the line number in the file (not the diff position). If you cannot determine the exact line,`,
        `set line to null and explain in the body.`,
        `Severity: blocker (must fix before merge), required (should fix), advisory (consider), nit (minor).`,
      ].join('\n'),
      { schema: FINDINGS_SCHEMA, label: `review:${dim}`, phase: 'Review', agentType: 'general-purpose' },
    )
  ),
)

// Collect all findings
const allFindings = reviewResults
  .filter(Boolean)
  .flatMap((r) => r.findings || [])
  .filter((f) => f && f.file)

log(`${allFindings.length} total finding(s) across ${activeDimensions.length} dimension(s)`)

const summary = allFindings.length === 0
  ? `**Author Review: ${pre.owner}/${pre.repo} #${pre.prNumber}**\n\n✅ No findings. The code looks clean across ${activeDimensions.join(', ')}.`
  : [
      `**Author Review: ${pre.owner}/${pre.repo} #${pre.prNumber}**`,
      ``,
      `Found ${allFindings.length} issue(s) across ${activeDimensions.length} dimension(s):`,
      ...['blocker', 'required', 'advisory', 'nit'].map(
        (s) => `- **${s}**: ${allFindings.filter((f) => f.severity === s).length}`,
      ),
      ``,
      `---`,
      ...allFindings.map((f, i) =>
        `${i + 1}. **${f.severity}** — \`${f.file}\`${f.line ? `:${f.line}` : ''} — ${f.title}\n   ${f.body}`,
      ),
    ].join('\n')

if (dryRun) {
  return {
    stage: 'dry-run', ok: true, prUrl: pre.prUrl, prNumber: pre.prNumber,
    dimensions: activeDimensions, findings: allFindings.length,
    summary,
    nextStep: `Dry run: ${allFindings.length} finding(s) collected. Re-run without --dryRun to post as PR review comments.`,
  }
}

if (allFindings.length === 0) {
  // Post a clean review (no inline comments, just summary)
  const result = await agent(
    `The PR review found no issues. Post a clean review on PR #${pre.prNumber} to record that author-review passed.\n\n`
    + `1. Post a PR review with event=COMMENT:\n`
    + `gh api repos/${pre.owner}/${pre.repo}/pulls/${pre.prNumber}/reviews --field event="COMMENT" --field body="${summary.replace(/"/g, '\\"')}"\n\n`
    + `Return { posted: true, reviewId: <id from response> } or an error message.`,
    { label: 'post-clean', phase: 'Post', schema: { type: 'object', additionalProperties: false, required: ['posted'], properties: { posted: { type: 'boolean' }, reviewId: { type: ['integer', 'null'] } } } },
  )
  return {
    stage: 'done', ok: true, prUrl: pre.prUrl, prNumber: pre.prNumber,
    mode: 'review', findings: 0, dimensions: activeDimensions,
    cleanReview: !!(result && result.posted),
    nextStep: `No issues found. PR #${pre.prNumber} passed author-review. Ready for merge — or the reviewee can run /opsx:address-review ${change || ''} for any remaining comments.`,
  }
}

// ---------------------------------------------------------------- Phase 3: Post inline PR review comments
phase('Post')
if (budget && budget.total && budget.remaining() < reserve) {
  return { stage: 'post', ok: false, reason: 'budget reserve reached before posting', prUrl: pre.prUrl, findings: allFindings }
}

const post = await agent(
  [
    `Post ${allFindings.length} review finding(s) as a PR review with inline comments on PR #${pre.prNumber}.`,
    `Repo: ${pre.owner}/${pre.repo}, head SHA: ${pre.headSha}.`,
    ``,
    `Write the summary to a file:`,
    `cat > /tmp/ar-summary.txt << 'SUMMARYEOF'\n${summary}\nSUMMARYEOF`,
    ``,
    `Build the comments JSON:`,
    `cat > /tmp/ar-comments.json << 'JSONEOF'\n${JSON.stringify(allFindings.map((f) => ({ path: f.file, line: f.line || 1, side: "RIGHT", body: `**${f.severity}**: ${f.title}\n\n${f.body}` })))}\nJSONEOF`,
    ``,
    `Post the review:`,
    `gh api -X POST repos/${pre.owner}/${pre.repo}/pulls/${pre.prNumber}/reviews \\`,
    `  -f event="COMMENT" \\`,
    `  -f body="$(cat /tmp/ar-summary.txt)" \\`,
    `  -f commit_id="${pre.headSha}" \\`,
    `  --field-json "comments=$(cat /tmp/ar-comments.json)"`,
    ``,
    `Verify: gh api repos/${pre.owner}/${pre.repo}/pulls/${pre.prNumber}/reviews --jq 'last | {state, id}'`,
    ``,
    `Return { posted: true, reviewId: <id>, commentsPosted: <count> }. On failure, return { posted: false, error: "<message>" }.`,
  ].join('\n'),
  {
    label: 'post-review',
    phase: 'Post',
    schema: {
      type: 'object', additionalProperties: false,
      required: ['posted'],
      properties: { posted: { type: 'boolean' }, reviewId: { type: ['integer', 'null'] }, commentsPosted: { type: 'integer' }, error: { type: 'string' } },
    },
    agentType: 'general-purpose',
  },
)

// ---------------------------------------------------------------- Phase 4: Summary
phase('Summary')
const result = {
  stage: 'done', ok: true, mode: 'review',
  prUrl: pre.prUrl, prNumber: pre.prNumber,
  findings: allFindings.length,
  dimensions: activeDimensions,
  posted: !!(post && post.posted),
  reviewId: post ? post.reviewId : null,
  commentsPosted: post ? post.commentsPosted || allFindings.length : 0,
  bySeverity: {
    blocker: allFindings.filter((f) => f.severity === 'blocker').length,
    required: allFindings.filter((f) => f.severity === 'required').length,
    advisory: allFindings.filter((f) => f.severity === 'advisory').length,
    nit: allFindings.filter((f) => f.severity === 'nit').length,
  },
  nextStep: post && post.posted
    ? `Posted ${(post.commentsPosted || allFindings.length)} inline comment(s) on PR #${pre.prNumber} (review ID: ${post.reviewId || '?'}).\n`
      + `**Lead**: Check the PR at ${pre.prUrl}, add any manual comments, then tell the reviewee.\n`
      + `**Reviewee**: Run /opsx:address-review ${change || '<change>'} to pick up the comments and fix them.`
    : `Review complete but posting failed${post ? ': ' + (post.error || 'unknown') : ''}. Findings summary:\n${summary}\n\nPost manually or retry.`,
}
if (post && !post.posted) {
  result.ok = false
  result.error = post.error || 'posting failed'
}
return result
