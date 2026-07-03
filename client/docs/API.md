# BlenderKit-Client API

> Generated from `internal/apispec` by `cmd/apidocgen`. Do not edit by hand.

**Client version:** `1.10.0` &nbsp;&nbsp; **Versioned prefix:** `/v1.10`

The Client is a local HTTP server (default port **62485**) that bridges BlenderKit DCC add-ons (Blender, Godot, and embedders such as Maya and Rhino) with the BlenderKit web service.

Most endpoints are registered twice: once under the bare path (e.g. `/report`) and once under the versioned prefix (e.g. `/v1.10/report`). Both are equivalent.

A machine-readable [OpenAPI 3.1 spec](openapi.json) is generated alongside this file. Import it into Postman/Insomnia, render it with Swagger UI/Redoc, or generate client SDKs from it.

## Endpoints by group

### core

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| GET | `/` |  |  | Client status page | — |
| POST | `/report` | ✓ |  | Poll for tasks (Blender) | `GetReportData` |
| GET, POST | `/shutdown` | ✓ |  | Shut down the Client | — |
| GET | `/debug` | ✓ |  | Network/debug diagnostics | — |

### login

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| GET | `/consumer/exchange/` |  |  | OAuth2 redirect landing | — |
| POST | `/refresh_token` | ✓ |  | Refresh access token | `RefreshTokenData` |
| POST | `/oauth2/verification_data` | ✓ |  | Store OAuth2 PKCE session | `OAuth2VerificationData` |
| POST | `/oauth2/logout` | ✓ | 🔑 | Log out / revoke tokens | `RefreshTokenData` |

### blender

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/blender/unsubscribe_addon` | ✓ |  | Unsubscribe a Blender add-on | `ReportData` |
| POST | `/blender/cancel_download` | ✓ |  | Cancel a download | `CancelDownloadData` |
| POST | `/blender/asset_download` | ✓ |  | Download an asset | `DownloadData` |
| POST | `/blender/asset_prxc_download` | ✓ |  | Download a proxy collection asset | `DownloadPrxcData` |
| POST | `/blender/asset_search` | ✓ |  | Search assets | `SearchTaskData` |
| POST | `/blender/asset_upload` | ✓ | 🔑 | Upload an asset | `AssetUploadRequestData` |

### host-agnostic

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/run_blender_script` | ✓ |  | Run a Python recipe in headless Blender | `RunBlenderScriptData` |

### profiles

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/profiles/download_gravatar_image` | ✓ |  | Download gravatar image | `FetchGravatarData` |
| POST | `/profiles/get_user_profile` | ✓ | 🔑 | Get user profile | `MinimalTaskData` |

### comments

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/comments/get_comments` | ✓ |  | Get asset comments | `GetCommentsData` |
| POST | `/comments/create_comment` | ✓ | 🔑 | Create a comment | `CreateCommentData` |
| POST | `/comments/feedback_comment` | ✓ | 🔑 | Like/dislike a comment | `FeedbackCommentTaskData` |
| POST | `/comments/mark_comment_private` | ✓ | 🔑 | Toggle comment privacy | `MarkCommentPrivateTaskData` |

### notifications

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/notifications/mark_notification_read` | ✓ | 🔑 | Mark notification read | `MarkNotificationReadTaskData` |

### ratings

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/ratings/get_bookmarks` | ✓ | 🔑 | Get bookmarks | `MinimalTaskData` |
| POST | `/ratings/get_rating` | ✓ | 🔑 | Get asset rating | `GetRatingData` |
| POST | `/ratings/send_rating` | ✓ | 🔑 | Send asset rating | `SendRatingData` |

### wrappers

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/wrappers/get_download_url` | ✓ |  | Resolve asset download URL | `DownloadData` |
| POST | `/wrappers/complete_upload_file_blocking` | ✓ | 🔑 | Complete a file upload (blocking) | `CompleteUploadFileBlockingData` |
| POST | `/wrappers/blocking_file_download` | ✓ |  | Download a file (blocking) | `BlockingFileDownloadTaskData` |
| POST | `/wrappers/blocking_request` | ✓ |  | Proxy a blocking HTTP request | `BlockingRequestData` |
| POST | `/wrappers/nonblocking_request` | ✓ |  | Proxy a non-blocking HTTP request | `NonblockingRequestTaskData` |

### bkclientjs

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| GET, OPTIONS | `/bkclientjs/status` | ✓ |  | Browser: Client status | — |
| POST, OPTIONS | `/bkclientjs/get_asset` | ✓ |  | Browser: request asset download | `bkclientjsDownloadData` |

### godot

| Method | Path | Versioned | Auth | Summary | Request body |
|---|---|:---:|:---:|---|---|
| POST | `/godot/report` | ✓ |  | Poll for tasks (Godot) | `Software` |
| POST | `/godot/unsubscribe_addon` | ✓ |  | Unsubscribe the Godot add-on | `ReportData` |

## Endpoint details

### core

#### `GET /`

Returns a small HTML page with the Client PID, version, platform, system ID and the add-on that started it. Any non-root path returns 404.

- **Handler:** `indexHandler`

#### `POST /report`

Primary polling endpoint for Blender add-ons. Subscribes the add-on on first call, refreshes the inactivity timer and returns the list of pending/finished/error tasks for the calling app. Rejects add-ons that do not send addon_version.

- **Handler:** `reportHandler`
- **Versioned alias:** `/v1.10/report`
- **Request body:** JSON `GetReportData` (Go struct in package main)

#### `GET / POST /shutdown`

Schedules a graceful exit of the Client process shortly after responding 200 OK.

- **Handler:** `shutdownHandler`
- **Versioned alias:** `/v1.10/shutdown`

#### `GET /debug`

Returns diagnostic information about the Client's network configuration, useful for troubleshooting connectivity and proxy issues.

- **Handler:** `DebugNetworkHandler`
- **Versioned alias:** `/v1.10/debug`

### login

#### `GET /consumer/exchange/`

Browser redirect target after the user logs in on blenderkit.com. Validates the OAuth2 code and state query parameters, exchanges the code for tokens and redirects the browser to the server's oauth-landing page. Intentionally unversioned to keep the server-side redirect URL simple.

- **Handler:** `consumerExchangeHandler`
- **Request notes:** Query parameters: code (authorization code), state (CSRF/session state).

#### `POST /refresh_token`

Refreshes the access token using a refresh token. On success a 'login' task is delivered to every connected app; on failure the apps are logged out.

- **Handler:** `RefreshTokenHandler`
- **Versioned alias:** `/v1.10/refresh_token`
- **Request body:** JSON `RefreshTokenData` (Go struct in package main)

#### `POST /oauth2/verification_data`

Stores the add-on's PKCE code_verifier and state so the later /consumer/exchange/ redirect can be verified.

- **Handler:** `OAuth2VerificationDataHandler`
- **Versioned alias:** `/v1.10/oauth2/verification_data`
- **Request body:** JSON `OAuth2VerificationData` (Go struct in package main)

#### `POST /oauth2/logout`

Revokes the API key and refresh token on the server and logs the user out of all connected apps.

- **Handler:** `OAuth2LogoutHandler`
- **Versioned alias:** `/v1.10/oauth2/logout`
- **Request body:** JSON `RefreshTokenData` (Go struct in package main)
- **Auth:** requires a logged-in BlenderKit API key

### blender

#### `POST /blender/unsubscribe_addon`

Cancels all running tasks for the app and removes it from the Client's task registry.

- **Handler:** `blenderUnsubscribeAddonHandler`
- **Versioned alias:** `/v1.10/blender/unsubscribe_addon`
- **Request body:** JSON `ReportData` (Go struct in package main)

#### `POST /blender/cancel_download`

Cancels an in-progress asset download task.

- **Handler:** `CancelDownloadHandler`
- **Versioned alias:** `/v1.10/blender/cancel_download`
- **Request body:** JSON `CancelDownloadData` (Go struct in package main)

#### `POST /blender/asset_download`

Starts an asynchronous asset download. Progress and result are reported back through /report tasks.

- **Handler:** `assetDownloadHandler`
- **Versioned alias:** `/v1.10/blender/asset_download`
- **Request body:** JSON `DownloadData` (Go struct in package main)

#### `POST /blender/asset_prxc_download`

Starts an asynchronous download of a proxy collection (prxc) asset.

- **Handler:** `assetPrxcDownloadHandler`
- **Versioned alias:** `/v1.10/blender/asset_prxc_download`
- **Request body:** JSON `DownloadPrxcData` (Go struct in package main)
- **Request notes:** Body embeds DownloadPrxcData plus an app_id field.

#### `POST /blender/asset_search`

Starts an asynchronous asset search. Results, including thumbnail downloads, are reported back through /report tasks.

- **Handler:** `assetSearchHandler`
- **Versioned alias:** `/v1.10/blender/asset_search`
- **Request body:** JSON `SearchTaskData` (Go struct in package main)

#### `POST /blender/asset_upload`

Starts an asynchronous asset upload. Progress and result are reported back through /report tasks.

- **Handler:** `assetUploadHandler`
- **Versioned alias:** `/v1.10/blender/asset_upload`
- **Request body:** JSON `AssetUploadRequestData` (Go struct in package main)
- **Auth:** requires a logged-in BlenderKit API key

### host-agnostic

#### `POST /run_blender_script`

Runs a Python recipe under headless Blender. Used by external embedders (e.g. the Rhino plug-in) and available for the add-on's own background-script needs.

- **Handler:** `runBlenderScriptHandler`
- **Versioned alias:** `/v1.10/run_blender_script`
- **Request body:** JSON `RunBlenderScriptData` (Go struct in package main)

### profiles

#### `POST /profiles/download_gravatar_image`

Downloads a user's gravatar/avatar image into the Client temp directory.

- **Handler:** `DownloadGravatarImageHandler`
- **Versioned alias:** `/v1.10/profiles/download_gravatar_image`
- **Request body:** JSON `FetchGravatarData` (Go struct in package main)

#### `POST /profiles/get_user_profile`

Fetches the logged-in user's BlenderKit profile.

- **Handler:** `GetUserProfileHandler`
- **Versioned alias:** `/v1.10/profiles/get_user_profile`
- **Request body:** JSON `MinimalTaskData` (Go struct in package main)
- **Auth:** requires a logged-in BlenderKit API key

### comments

#### `POST /comments/get_comments`

Fetches comments for an asset.

- **Handler:** `GetCommentsHandler`
- **Versioned alias:** `/v1.10/comments/get_comments`
- **Request body:** JSON `GetCommentsData` (Go struct in package main)

#### `POST /comments/create_comment`

Posts a new comment (or reply) on an asset.

- **Handler:** `CreateCommentHandler`
- **Versioned alias:** `/v1.10/comments/create_comment`
- **Request body:** JSON `CreateCommentData` (Go struct in package main)
- **Auth:** requires a logged-in BlenderKit API key

#### `POST /comments/feedback_comment`

Sends feedback (like or dislike) on a comment.

- **Handler:** `FeedbackCommentHandler`
- **Versioned alias:** `/v1.10/comments/feedback_comment`
- **Request body:** JSON `FeedbackCommentTaskData` (Go struct in package main)
- **Auth:** requires a logged-in BlenderKit API key

#### `POST /comments/mark_comment_private`

Marks a comment as private or public.

- **Handler:** `MarkCommentPrivateHandler`
- **Versioned alias:** `/v1.10/comments/mark_comment_private`
- **Request body:** JSON `MarkCommentPrivateTaskData` (Go struct in package main)
- **Auth:** requires a logged-in BlenderKit API key

### notifications

#### `POST /notifications/mark_notification_read`

Marks a server notification as read.

- **Handler:** `MarkNotificationReadHandler`
- **Versioned alias:** `/v1.10/notifications/mark_notification_read`
- **Request body:** JSON `MarkNotificationReadTaskData` (Go struct in package main)
- **Auth:** requires a logged-in BlenderKit API key

### ratings

#### `POST /ratings/get_bookmarks`

Fetches the user's bookmarked assets.

- **Handler:** `GetBookmarksHandler`
- **Versioned alias:** `/v1.10/ratings/get_bookmarks`
- **Request body:** JSON `MinimalTaskData` (Go struct in package main)
- **Auth:** requires a logged-in BlenderKit API key

#### `POST /ratings/get_rating`

Fetches the user's ratings for an asset.

- **Handler:** `GetRatingHandler`
- **Versioned alias:** `/v1.10/ratings/get_rating`
- **Request body:** JSON `GetRatingData` (Go struct in package main)
- **Auth:** requires a logged-in BlenderKit API key

#### `POST /ratings/send_rating`

Submits a rating for an asset. Only POST is accepted.

- **Handler:** `SendRatingHandler`
- **Versioned alias:** `/v1.10/ratings/send_rating`
- **Request body:** JSON `SendRatingData` (Go struct in package main)
- **Auth:** requires a logged-in BlenderKit API key

### wrappers

#### `POST /wrappers/get_download_url`

Blocking helper that resolves the direct download URL and filename for an asset file.

- **Handler:** `GetDownloadURLWrapper`
- **Versioned alias:** `/v1.10/wrappers/get_download_url`
- **Request body:** JSON `DownloadData` (Go struct in package main)

#### `POST /wrappers/complete_upload_file_blocking`

Blocking helper that completes a multi-part file upload.

- **Handler:** `CompleteUploadFileBlocking`
- **Versioned alias:** `/v1.10/wrappers/complete_upload_file_blocking`
- **Request body:** JSON `CompleteUploadFileBlockingData` (Go struct in package main)
- **Auth:** requires a logged-in BlenderKit API key

#### `POST /wrappers/blocking_file_download`

Blocking helper that downloads a file and returns once finished.

- **Handler:** `BlockingFileDownloadHandler`
- **Versioned alias:** `/v1.10/wrappers/blocking_file_download`
- **Request body:** JSON `BlockingFileDownloadTaskData` (Go struct in package main)

#### `POST /wrappers/blocking_request`

Blocking helper that performs an arbitrary HTTP request to the BlenderKit server through the Client's configured HTTP client and returns the response.

- **Handler:** `BlockingRequestHandler`
- **Versioned alias:** `/v1.10/wrappers/blocking_request`
- **Request body:** JSON `BlockingRequestData` (Go struct in package main)

#### `POST /wrappers/nonblocking_request`

Schedules an arbitrary HTTP request to the BlenderKit server; the response is reported back through /report tasks.

- **Handler:** `NonblockingRequestHandler`
- **Versioned alias:** `/v1.10/wrappers/nonblocking_request`
- **Request body:** JSON `NonblockingRequestTaskData` (Go struct in package main)

### bkclientjs

#### `GET / OPTIONS /bkclientjs/status`

CORS-enabled endpoint used by bkclient.js in the web browser to read the Client version and the list of connected softwares. Supports OPTIONS preflight.

- **Handler:** `bkclientjsStatusHandler`
- **Versioned alias:** `/v1.10/bkclientjs/status`

#### `POST / OPTIONS /bkclientjs/get_asset`

CORS-enabled endpoint used by bkclient.js to ask a connected software to download an asset. Supports OPTIONS preflight.

- **Handler:** `bkclientjsGetAssetHandler`
- **Versioned alias:** `/v1.10/bkclientjs/get_asset`
- **Request body:** JSON `bkclientjsDownloadData` (Go struct in package main)

### godot

#### `POST /godot/report`

Polling endpoint for the Godot add-on. Subscribes the app on first call and returns a SoftwareResponse with the Client version, a connection message and pending tasks.

- **Handler:** `godotReportHandler`
- **Versioned alias:** `/v1.10/godot/report`
- **Request body:** JSON `Software` (Go struct in package main)

#### `POST /godot/unsubscribe_addon`

Cancels all running tasks for the Godot app and removes it from the Client's task registry.

- **Handler:** `godotUnsubscribeAddonHandler`
- **Versioned alias:** `/v1.10/godot/unsubscribe_addon`
- **Request body:** JSON `ReportData` (Go struct in package main)

