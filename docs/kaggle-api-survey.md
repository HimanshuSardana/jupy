# Kaggle API survey — what a TUI can display

Surveyed against: `kaggle` pip 2.2.4 + `kagglesdk` 0.1.37 (local at
`~/.local/share/uv/tools/kaggle/lib/python3.12/site-packages/`), plus live
unauthenticated probes of `www.kaggle.com/api/v1/*` and `api.kaggle.com/v1/*`.
No Go client exists — `kaggle-api`'s swagger URL is 404 and there is no
published Go SDK, so a Go TUI hand-rolls the HTTP calls.

## 1. Two API layers

| | Legacy REST | Modern RPC (proto-JSON) |
|---|---|---|
| Base | `https://www.kaggle.com/api/v1/<path>?query` | `https://api.kaggle.com/v1/<pkg.Service>/<Method>` |
| Verb | GET (POST/PUT for writes) | POST always, JSON body = proto message |
| Schema | Java/Jackson models, fields duplicated as `x` + `xNullable` + `hasX` | proto field names, `pageToken` + `totalResults` |
| Pagination | `page` (1-based) + `pageSize`; some endpoints return a bare array | `pageToken` + `pageSize`, echoed `nextPageToken` |
| Auth | HTTP Basic `username:kaggle.json key` | `Authorization: Bearer <token>` |
| Schema source | string paths baked into `kagglesdk/**/types/*.py` (`endpoint()`) | `kagglesdk/**/services/*.py` (`client.call(service, method, ...)`) |

Both are live. Recommendation for the TUI: **legacy REST for all read
panels** (paths and query params map 1:1 onto screens, debuggable with curl),
and the modern RPC only where nothing else exists (kernel sessions, episode
replay/logs, benchmark runs) — or skip writes entirely for v1.

Live probe (no credentials):

```
200  datasets/list                200  models/list
200  datasets/list/{owner}/{slug} 200  discussions/topics/list
200  discussions/forums/list      200  competitions/get/{ref}
200  competitions/{ref}/topics
401  competitions/list            401  kernels/list
401  competitions/{ref}/leaderboard/view
401  competitions/submissions/list/{ref}
```

`competitions/list` and `kernels/list` return 401 even for public data — those
two panels are gated behind a key no matter what. Anonymous mode would show
datasets / models / forums / competition *detail by name* only.

## 2. Auth (Go side)

- `~/.kaggle/kaggle.json` → `{"username","key"}`, HTTP Basic. Works on both layers.
- `KAGGLE_API_TOKEN` env or `kaggle auth print-access-token` → Bearer (RPC layer only).
- `~/.kaggle/credentials.json` → `{refresh_token, access_token, access_token_expiration (12h), username, scopes}`, written by `kaggle auth login` (OAuth2 + PKCE S256, authorize at `/api/v1/oauth2/authorize`).
- Plan: Basic from `kaggle.json`; fall back to bearer; refresh OAuth when expired. Show auth state in a statusline (signed out = read-only anonymous panels).

## 3. Displayable inventory, per screen

### Competitions
- list: `/api/v1/competitions/list?group=&category=&sortBy=&search=&page=&pageSize=` — **401 anonymous**
- row (`ApiCompetition`): `ref`, `title`, `url`, `description`, `organizationName`, `organizationRef`, `category`, `reward` (prize text), `tags[]`, `deadline`, `mergerDeadline`, `newEntrantDeadline`, `enabledDate`, `dateCreated`, `kernelCount`, `teamCount`, `maxTeamSize`, `maxDailySubmissions`, `evaluationMetric`, `awardsPoints`, `isKernelsSubmissionsOnly`, `submissionsDisabled`, `thumbnailImageUrl`, `hostName`, `userHasEntered`, `userRank`
- filters: `CompetitionListTab` (General/Entered/Community/Hosted/Unlaunched/Everything), `HostSegment` (Featured/Research/Recruitment/Playground/Getting Started/Masters/Community/Analytics), `CompetitionSortBy` (Grouped/Best/Prize/Earliest deadline/Latest deadline/#Teams/Relevance/Recently created)
- detail: `/competitions/get/{ref}` (same shape)
- data files: `/competitions/data/list/{ref}` → `ApiCompetitionDataFile{name, totalBytes}`; tree: `/competitions/{ref}/data-tree/list/`; summary: `/competitions/data/summary/{ref}`
- leaderboard: `/competitions/{ref}/leaderboard/view?pageSize=&pageToken=` → `ApiLeaderboardSubmission{teamId, teamName, submissionDate, score}` — **401 anon**
- submissions: `/competitions/submissions/list/{ref}?sortBy=&group=&page=&pageSize=` → `ApiSubmission{ref, date, description, errorDescription, fileName, publicScore, privateScore, status, submittedBy, submittedByRef, teamName, totalBytes, url}`; sorts: Date/Name/Private score/Public score; groups: All/Successful/Selected
- submission limits: `/competitions/{ref}/submissions/limits` → `{numToday, numTotal, numAllowedNow, limitedByTotal}`
- hosts/judges: `.../hosts`, `.../judges`; pages: `.../pages`; solution status: `.../solution`
- topics + messages: `.../topics`, `.../topics/{topicId}/messages`
- new-ish: episodes/replay/agent logs (agent-based competitions), launch/init/settings (host-only)

### Datasets
- list: `/api/v1/datasets/list?page=&pageSize=&search=&sortBy=&size=&fileType=&license=&group=&user=&minSize=&maxSize=&tagIds=` — **works anonymous**; returns a **bare JSON array**
- row (`ApiDataset`): `ref`, `title`, `subtitle`, `description`, `ownerName`/`ownerRef`, `creatorName`/`creatorUrl`, `totalBytes`, `lastUpdated`, `downloadCount`, `viewCount`, `voteCount`, `kernelCount`, `topicCount`, `usabilityRating` (0–1 float), `isPrivate`, `isFeatured`, `licenseName`, `currentVersionNumber`, `thumbnailImageUrl`, `tags[{ref,name,fullPath,competitionCount,datasetCount,scriptCount,totalCount}]`
- filters: `DatasetSelectionGroup` (Public/My/User/Featured/Upvoted/Bookmarked/Collaboration/…) , `DatasetSortBy` (Hottest/Votes/Updated/Active/Published/Relevance/Usability/Downloads/Notebook count/Last viewed), `DatasetSizeGroup` (Small/Medium/Large), `DatasetFileTypeGroup` (CSV/SQLite/JSON/BigQuery/Parquet), `DatasetLicenseGroup` (CC/GPL/ODB/Other)
- detail: `/datasets/view/{owner}/{slug}` — full record with `files[]` and `versions[]`
- files: `/datasets/list/{owner}/{slug}` → `ApiDatasetFile{ref, name, creationDate, description, fileType, url, totalBytes, columns[]}`; tree: `/datasets/list-tree/{owner}/{slug}`
- metadata YAML: `/datasets/metadata/{owner}/{slug}`; status: `/datasets/status/{owner}/{slug}`; file summary: `/datasets/files/summary/{owner}/{slug}`
- topics: `/datasets/{owner}/{slug}/topics/list`

### Notebooks / kernels
- list: `/api/v1/kernels/list?page=&pageSize=&group=&kernelType=&language=&outputType=&sortBy=&search=&user=&competition=&dataset=&parentKernel=` — **401 anonymous**
- row (`ApiKernelMetadata`): `ref`, `id`, `title`, `author`, `slug`, `lastRunTime`, `language`, `kernelType`, `isPrivate`, `enableGpu`, `enableTpu`, `enableInternet`, `machineShape`, `dockerImage`, `categoryIds[]`, `datasetDataSources[]`, `kernelDataSources[]`, `competitionDataSources[]`, `modelDataSources[]`, `totalVotes`, `currentVersionNumber`
- filters: `KernelsListViewType` (Profile/Upvoted/Everyone/Collaboration/Fork/Bookmarked/Recently viewed/Public+private), `KernelsListSortType` (Hotness/Comment count/Date created/Date run/Relevance/Score asc/Score desc/View count/Vote count)
- files: `/kernels/list/{owner}/{slug}` → `ApiListKernelFilesItem{name, size, creationDate}`
- source pull: `/kernels/pull?userName=&kernelSlug=` → `ApiKernelBlob{source, language, kernelType, slug}`
- output: `/kernels/output/list/{owner}/{slug}` → `{files[{name,size,creationDate}], log, nextPageToken}` (log is a big text blob → scrollable pane)
- session status: `/kernels/status?userName=&kernelSlug=` → `KernelWorkerStatus` (QUEUED/RUNNING/COMPLETE/ERROR/CANCEL_REQUESTED/CANCEL_ACKNOWLEDGED/NEW_SCRIPT) + `failureMessage` — a live progress spinner/status line
- logs: `/kernels/logs/stream/...` streaming text
- accelerator quota: `/kernels/quota` → per-accelerator `ApiAcceleratorQuota{timeUsed, timeReserved, totalTimeAllowed, minimumTimeAllowed, isPayToScaleEnabled, hasEverRun}`

### Models
- list: `/api/v1/models/list?search=&sortBy=&owner=&pageSize=&pageToken=` — **works anonymous**
- row (`ApiModel`): `ref`, `title`, `subtitle`, `description`, `author`, `authorImageUrl`, `slug`, `isPrivate`, `publishTime`, `updateTime`, `provenanceSources`, `url`, `voteCount`, `tags[]`, `instances[]`, `modelVersionLinks[]`
- sort: `ListModelsOrderBy` (Hotness/Downloads/Votes/Notebook count/Publish/Create/Update/View time)
- instances/versions: `/models/{owner}/{slug}/instances`, `.../instances/{instance}/versions`, `.../versions/{v}/files` → `ApiModelInstance{slug, framework, fineTunable, overview, usage, downloadUrl, licenseName, totalUncompressedBytes, modelInstanceType, trainingData[]}`, `ModelInstanceVersion{versionNumber, framework, sigstoreState, isPrivate, thumbnailUrl}`
- frameworks enum is big (`ModelFramework`: TF1/TF2/TF-Lite/TF-JS/PyTorch/JAX/Flax/Pax/MaxText/Gemma CPP/GGML/GGUF/ONNX/Keras/Transformers/vLLM/Triton/… ) — good filter sidebar
- metrics: `GetModelMetricsResponse{metrics[]}` (activity time series → could feed a sparkline)

### Discussions / forums
- forums: `/api/v1/discussions/forums/list` → `{id, name, url, subtitle}` — anonymous OK
- topics: `/api/v1/discussions/topics/list?category=&group=&sortBy=&searchQuery=&forumSlug=&pageSize=` → `ApiDiscussionTopic{id, title, url, postDate, lastCommentDate, authorName, authorUrl, votes, commentCount, forumName, forumId, content}` (+ `totalCount`, `nextPageToken`)
- scoped topics: `/datasets/{o}/{s}/topics/list`, `/models/…`, `/kernels/…`, `/competitions/{ref}/topics`, `/benchmarks/…`
- filters: `TopicListCategory` (All/Forums/Competitions/Datasets/Competition write-ups/Models/Benchmarks), `TopicListGroup` (All/Owned/Upvoted/Bookmarked/My activity/Drafts), `TopicListSortBy` (Hot/Top/New/Recent/Active/Relevance)
- topic + comments: `/discussions/{id}/get`, `/discussions/{topicId}/comments?pageSize=&pageToken=` → `ApiDiscussionComment{id, authorName, authorUrl, postDate, content, votes, replies[]}` (recursive)
- `CommentAuthorType` (Host/Dataset creator/Admin/Model creator/Judge/…) → badge column

### Unified search (one global "search everything" screen)
- Legacy path `search/list`, RPC `search.SearchApiService/ListEntities`.
- `ListEntitiesRequest{filters, canonicalOrderBy, competitionsOrderBy, datasetsOrderBy, kernelsOrderBy, modelsOrderBy, discussionsOrderBy, usersOrderBy, pageToken, pageSize, skip}`
- `ListEntitiesFilters{query, listType, privacy (All/Private/Public), ownerType (Owns/Collaborations/Shared with me), documentTypes[], competitionFilters, datasetFilters, discussionFilters, kernelFilters, modelFilters, userFilters}`
- result row `ListEntitiesDocument{id, documentType, title, imageUrl, createTime, updateTime, isPrivate, votes, ownerUser{userName, displayName, thumbnailUrl, tier}, ownerOrganization{name, slug}, slug}` + one typed sub-document per `DocumentType` (Competition/Dataset/Kernel/Model/Discussion/User/Benchmark/Blog/Course/Tutorial)
- this is the single best endpoint for a "cmd-K omnibox": one call, mixed result kinds, typed facets for the detail pane

### Benchmarks (agent/LLM evals)
- tasks: `/api/v1/benchmarks/tasks/list`, `.../tasks/{owner.slug}`, runs `.../tasks/runs/list`, per-run logs/output, quota, publish
- `ApiBenchmarkTask{slug, url, creationState, createTime, isPublic, sourceKernelId, isBackingNotebookPublished, options, source}`; run states QUEUED/RUNNING/COMPLETED/ERRORED/SCORE_PENDING → nice status column
- leaderboard: `/benchmarks/{owner}/{slug}/versions/{n}/leaderboard` → `LeaderboardRow`

### Account / per-user widgets
- accelerator quota (`/kernels/quota`), submission limits per competition, benchmark quota, group memberships (`users.GroupApiService/*`), `kaggle quota` CLI equivalent
- user rows via search: `ApiSearchUsersDocument{grandmasterTierLevel, userLocation, occupationOrganizationName, competitionRanking, competitionPoints, kernelRanking, kernelPoints, datasetRanking, datasetPoints}` → profile/leaderboard card without a dedicated profile endpoint

## 4. Shapes that will bite a Go client

1. **Duplicated nullable fields** on legacy responses: every optional field ships as `x` + `xNullable` + `hasX` (e.g. `title`, `titleNullable`, `hasTitle`). Decode into `*T` on `x` and ignore the rest, or you will write three struct fields per field.
2. **Mixed list envelopes**: `datasets/list` returns a bare `[...]`; `models/list`, `discussions/topics/list`, `datasets/list/{o}/{s}` return `{"x": [...], "nextPageToken": ...}`. No uniform list wrapper.
3. **Timestamps** are RFC3339 strings in legacy, sometimes missing entirely; `ApiCompetition.deadline` can be absent for unlaunched competitions.
4. **`usabilityRating`** is a 0–1 float, not a percentage — format at render time.
5. **Markdown**: `description` and `content` are markdown; wire a minimal renderer (headers, bold, code, lists, links) or strip to plain text.
6. **Images**: only `thumbnailImageUrl` / `imageUrl` URLs, no raster endpoints — a text TUI shows the URL or skips; sixel/kitty-graphics is optional polish.
7. **Pagination**: page-int endpoints cap around 100/page and silently clamp; token endpoints should store `nextPageToken` per panel, not per app.
8. **No endpoints for**: notifications, personal medal counts, private messages, user profile summary (only search-derived cards), dataset column stats beyond `ApiDatasetFile.columns`.
9. **ncurses**: Go bindings to ncurses are thin and mostly unmaintained; `tcell` (termbox successor) or `gocui`/`bubbletea` give the same grid+key handling with portable terminal capability detection. Worth deciding before writing widgets.

## 5. Suggested screen map

```
Competitions   list | filters | detail | data files | leaderboard | submissions | limits | topics
Datasets       list | filters | detail | files(tree) | metadata | versions | topics
Notebooks      list | filters | detail(src+output) | files | logs | status | quota
Models         list | filters | detail | instances | versions | files | metrics
Discussions    forums | topics (per scope) | topic + comment tree
Search         one omnibox over all document types
Quota          GPU/TPU time used/reserved/allowed + submission limits
```

Read-only v1 is achievable with: legacy `datasets/*`, `models/*`, `discussions/*`,
`competitions/get`, `competitions/*/topics`, `search/list` — everything else
needs credentials.
