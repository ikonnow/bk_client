// Package apispec is the single source of truth for the Blendkit-Client HTTP API.
//
// The Routes() registry below describes every endpoint the Client exposes. It is
// consumed by:
//   - cmd/apidocgen, which renders docs/openapi.json (OpenAPI 3.1) and docs/API.md.
//   - the drift test in apidoc_test.go (package main), which guarantees this
//     registry stays in sync with the real mux.HandleFunc registrations in main.go.
//
// Adding, removing or renaming an endpoint in main.go without updating this
// registry will fail the drift test. This keeps the published API documentation
// always matching the actual server behaviour, without changing runtime behaviour.
package apispec

// Route describes a single HTTP endpoint exposed by the Client.
type Route struct {
	// Path is the canonical (unversioned) route, e.g. "/report".
	Path string
	// Methods are the accepted HTTP methods, e.g. []string{"POST"}.
	Methods []string
	// Versioned reports whether the route is also registered under the
	// versioned prefix "/vX.Y" (e.g. "/v1.9/report"). Most routes are.
	Versioned bool
	// Tag groups related endpoints in the generated documentation.
	Tag string
	// Summary is a short one-line description.
	Summary string
	// Description is a longer explanation, used in OpenAPI and Markdown.
	Description string
	// Handler is the name of the Go handler function in package main.
	Handler string
	// RequestType is the Go struct name of the JSON request body, or ""
	// when the endpoint takes no JSON body.
	RequestType string
	// RequestNote documents non-JSON inputs (e.g. query parameters) or other
	// request details that the struct name alone does not convey.
	RequestNote string
	// RequiresAPIKey reports whether a logged-in Blendkit API key is needed
	// for the endpoint to do useful work.
	RequiresAPIKey bool
	// Deprecated marks routes kept only for backward compatibility. New code
	// should use the universal replacement noted in the Description.
	Deprecated bool
}

// Tags are the endpoint groups in the order they should appear in docs.
var Tags = []string{
	"core",
	"settings",
	"login",
	"assets",
	"addons",
	"host-agnostic",
	"profiles",
	"comments",
	"notifications",
	"ratings",
	"wrappers",
	"bkclientjs",
	"godot",
	"deprecated",
}

// Routes returns the full registry of Client HTTP endpoints.
//
// Returns:
//
//	A slice of Route describing every endpoint exposed by the Client.
func Routes() []Route {
	return []Route{
		// CORE
		{
			Path: "/", Methods: []string{"GET"}, Versioned: false, Tag: "core",
			Summary:     "Client status page",
			Description: "Returns a small HTML page with the Client PID, version, platform, system ID and the add-on that started it. Any non-root path returns 404.",
			Handler:     "indexHandler",
		},
		{
			Path: "/report", Methods: []string{"POST"}, Versioned: true, Tag: "core",
			Summary:     "Poll for tasks (Blender)",
			Description: "Primary polling endpoint for Blender add-ons. Subscribes the add-on on first call, refreshes the inactivity timer and returns the list of pending/finished/error tasks for the calling app. Rejects add-ons that do not send addon_version.",
			Handler:     "reportHandler", RequestType: "GetReportData",
		},
		{
			Path: "/shutdown", Methods: []string{"GET", "POST"}, Versioned: true, Tag: "core",
			Summary:     "Shut down the Client",
			Description: "Schedules a graceful exit of the Client process shortly after responding 200 OK.",
			Handler:     "shutdownHandler",
		},
		{
			Path: "/debug", Methods: []string{"GET"}, Versioned: true, Tag: "core",
			Summary:     "Network/debug diagnostics",
			Description: "Returns diagnostic information about the Client's network configuration, useful for troubleshooting connectivity and proxy issues.",
			Handler:     "DebugNetworkHandler",
		},
		{
			Path: "/dev", Methods: []string{"GET"}, Versioned: true, Tag: "core",
			Summary:     "Developer test dashboard",
			Description: "Serves a self-contained, same-origin HTML page with buttons to call the Client's endpoints and view their raw JSON responses. Because it is served by the Client itself, requests are same-origin and the settings endpoints (which emit no CORS headers) work directly from the browser. Manual-testing aid, not a production UI.",
			Handler:     "devDashboardHandler",
		},

		// SETTINGS - the Client is the source of truth; plugins sync from it.
		{
			Path: "/settings/get", Methods: []string{"GET", "POST"}, Versioned: true, Tag: "settings",
			Summary:     "Get Client settings",
			Description: "Returns the current settings snapshot (shared settings, global and per-plugin variables) for the running Client version, together with a monotonically increasing revision. Plugins must sync to this: the same snapshot is also broadcast on every /report response, so plugins apply whenever the revision grows.",
			Handler:     "getSettingsHandler",
		},
		{
			Path: "/settings/set", Methods: []string{"POST"}, Versioned: true, Tag: "settings",
			Summary:     "Change shared settings",
			Description: "Applies a change to the shared settings (only the fields present in the body are modified), bumps the revision and returns the new snapshot. The change is broadcast to every connected plugin on their next /report poll.",
			Handler:     "setSettingsHandler", RequestType: "SetSettingsData",
		},
		{
			Path: "/settings/set_variable", Methods: []string{"POST"}, Versioned: true, Tag: "settings",
			Summary:     "Store a variable",
			Description: "Stores a free-form variable/value pair on behalf of a plugin. An empty 'plugin' stores it globally (without plugin association); a non-empty 'plugin' namespaces it under that plugin name (e.g. blender -> executable). Bumps the revision and returns the new snapshot; the change is broadcast on the next /report poll.",
			Handler:     "setVariableHandler", RequestType: "SetVariableData",
		},

		// LOGIN / OAUTH2
		{
			Path: "/consumer/exchange/", Methods: []string{"GET"}, Versioned: false, Tag: "login",
			Summary:     "OAuth2 redirect landing",
			Description: "Browser redirect target after the user logs in on blendkit.com. Validates the OAuth2 code and state query parameters, exchanges the code for tokens and redirects the browser to the server's oauth-landing page. Intentionally unversioned to keep the server-side redirect URL simple.",
			Handler:     "consumerExchangeHandler",
			RequestNote: "Query parameters: code (authorization code), state (CSRF/session state).",
		},
		{
			Path: "/refresh_token", Methods: []string{"POST"}, Versioned: true, Tag: "login",
			Summary:     "Refresh access token",
			Description: "Refreshes the access token using a refresh token. On success a 'login' task is delivered to every connected app; on failure the apps are logged out.",
			Handler:     "RefreshTokenHandler", RequestType: "RefreshTokenData",
		},
		{
			Path: "/oauth2/verification_data", Methods: []string{"POST"}, Versioned: true, Tag: "login",
			Summary:     "Store OAuth2 PKCE session",
			Description: "Stores the add-on's PKCE code_verifier and state so the later /consumer/exchange/ redirect can be verified.",
			Handler:     "OAuth2VerificationDataHandler", RequestType: "OAuth2VerificationData",
		},
		{
			Path: "/oauth2/logout", Methods: []string{"POST"}, Versioned: true, Tag: "login",
			Summary:     "Log out / revoke tokens",
			Description: "Revokes the API key and refresh token on the server and logs the user out of all connected apps.",
			Handler:     "OAuth2LogoutHandler", RequestType: "RefreshTokenData", RequiresAPIKey: true,
		},

		// ASSETS - universal, host-agnostic asset operations. Any plugin
		// (Blender, Godot, embedders) may call these.
		{
			Path: "/assets/search", Methods: []string{"POST"}, Versioned: true, Tag: "assets",
			Summary:     "Search assets",
			Description: "Starts an asynchronous asset search. Results, including thumbnail downloads, are reported back through /report tasks.",
			Handler:     "assetSearchHandler", RequestType: "SearchTaskData",
		},
		{
			Path: "/assets/download", Methods: []string{"POST"}, Versioned: true, Tag: "assets",
			Summary:     "Download an asset",
			Description: "Starts an asynchronous asset download. Progress and result are reported back through /report tasks.",
			Handler:     "assetDownloadHandler", RequestType: "DownloadData",
		},
		{
			Path: "/assets/download_prxc", Methods: []string{"POST"}, Versioned: true, Tag: "assets",
			Summary:     "Download a proxy collection asset",
			Description: "Starts an asynchronous download of a proxy collection (prxc) asset.",
			Handler:     "assetPrxcDownloadHandler", RequestType: "DownloadPrxcData",
			RequestNote: "Body embeds DownloadPrxcData plus an app_id field.",
		},
		{
			Path: "/assets/upload", Methods: []string{"POST"}, Versioned: true, Tag: "assets",
			Summary:     "Upload an asset",
			Description: "Starts an asynchronous asset upload. Progress and result are reported back through /report tasks.",
			Handler:     "assetUploadHandler", RequestType: "AssetUploadRequestData", RequiresAPIKey: true,
		},
		{
			Path: "/assets/cancel_download", Methods: []string{"POST"}, Versioned: true, Tag: "assets",
			Summary:     "Cancel a download",
			Description: "Cancels an in-progress asset download task.",
			Handler:     "CancelDownloadHandler", RequestType: "CancelDownloadData",
		},

		// ADD-ONS - universal add-on lifecycle.
		{
			Path: "/addons/unsubscribe", Methods: []string{"POST"}, Versioned: true, Tag: "addons",
			Summary:     "Unsubscribe an add-on",
			Description: "Cancels all running tasks for the calling app and removes it from the Client's task registry. Host-agnostic: works for any subscribed software (Blender, Godot, embedders).",
			Handler:     "unsubscribeAddonHandler", RequestType: "ReportData",
		},

		// HOST-AGNOSTIC
		{
			Path: "/run_blender_script", Methods: []string{"POST"}, Versioned: true, Tag: "host-agnostic",
			Summary:     "Run a Python recipe in headless Blender",
			Description: "Runs a Python recipe under headless Blender. Used by external embedders (e.g. the Rhino plug-in) and available for the add-on's own background-script needs.",
			Handler:     "runBlenderScriptHandler", RequestType: "RunBlenderScriptData",
		},

		// PROFILES
		{
			Path: "/profiles/download_gravatar_image", Methods: []string{"POST"}, Versioned: true, Tag: "profiles",
			Summary:     "Download gravatar image",
			Description: "Downloads a user's gravatar/avatar image into the Client temp directory.",
			Handler:     "DownloadGravatarImageHandler", RequestType: "FetchGravatarData",
		},
		{
			Path: "/profiles/get_user_profile", Methods: []string{"POST"}, Versioned: true, Tag: "profiles",
			Summary:     "Get user profile",
			Description: "Fetches the logged-in user's Blendkit profile.",
			Handler:     "GetUserProfileHandler", RequestType: "MinimalTaskData", RequiresAPIKey: true,
		},

		// COMMENTS
		{
			Path: "/comments/get_comments", Methods: []string{"POST"}, Versioned: true, Tag: "comments",
			Summary:     "Get asset comments",
			Description: "Fetches comments for an asset.",
			Handler:     "GetCommentsHandler", RequestType: "GetCommentsData",
		},
		{
			Path: "/comments/create_comment", Methods: []string{"POST"}, Versioned: true, Tag: "comments",
			Summary:     "Create a comment",
			Description: "Posts a new comment (or reply) on an asset.",
			Handler:     "CreateCommentHandler", RequestType: "CreateCommentData", RequiresAPIKey: true,
		},
		{
			Path: "/comments/feedback_comment", Methods: []string{"POST"}, Versioned: true, Tag: "comments",
			Summary:     "Like/dislike a comment",
			Description: "Sends feedback (like or dislike) on a comment.",
			Handler:     "FeedbackCommentHandler", RequestType: "FeedbackCommentTaskData", RequiresAPIKey: true,
		},
		{
			Path: "/comments/mark_comment_private", Methods: []string{"POST"}, Versioned: true, Tag: "comments",
			Summary:     "Toggle comment privacy",
			Description: "Marks a comment as private or public.",
			Handler:     "MarkCommentPrivateHandler", RequestType: "MarkCommentPrivateTaskData", RequiresAPIKey: true,
		},

		// NOTIFICATIONS
		{
			Path: "/notifications/mark_notification_read", Methods: []string{"POST"}, Versioned: true, Tag: "notifications",
			Summary:     "Mark notification read",
			Description: "Marks a server notification as read.",
			Handler:     "MarkNotificationReadHandler", RequestType: "MarkNotificationReadTaskData", RequiresAPIKey: true,
		},

		// RATINGS
		{
			Path: "/ratings/get_bookmarks", Methods: []string{"POST"}, Versioned: true, Tag: "ratings",
			Summary:     "Get bookmarks",
			Description: "Fetches the user's bookmarked assets.",
			Handler:     "GetBookmarksHandler", RequestType: "MinimalTaskData", RequiresAPIKey: true,
		},
		{
			Path: "/ratings/get_rating", Methods: []string{"POST"}, Versioned: true, Tag: "ratings",
			Summary:     "Get asset rating",
			Description: "Fetches the user's ratings for an asset.",
			Handler:     "GetRatingHandler", RequestType: "GetRatingData", RequiresAPIKey: true,
		},
		{
			Path: "/ratings/send_rating", Methods: []string{"POST"}, Versioned: true, Tag: "ratings",
			Summary:     "Send asset rating",
			Description: "Submits a rating for an asset. Only POST is accepted.",
			Handler:     "SendRatingHandler", RequestType: "SendRatingData", RequiresAPIKey: true,
		},

		// WRAPPERS
		{
			Path: "/wrappers/get_download_url", Methods: []string{"POST"}, Versioned: true, Tag: "wrappers",
			Summary:     "Resolve asset download URL",
			Description: "Blocking helper that resolves the direct download URL and filename for an asset file.",
			Handler:     "GetDownloadURLWrapper", RequestType: "DownloadData",
		},
		{
			Path: "/wrappers/complete_upload_file_blocking", Methods: []string{"POST"}, Versioned: true, Tag: "wrappers",
			Summary:     "Complete a file upload (blocking)",
			Description: "Blocking helper that completes a multi-part file upload.",
			Handler:     "CompleteUploadFileBlocking", RequestType: "CompleteUploadFileBlockingData", RequiresAPIKey: true,
		},
		{
			Path: "/wrappers/blocking_file_download", Methods: []string{"POST"}, Versioned: true, Tag: "wrappers",
			Summary:     "Download a file (blocking)",
			Description: "Blocking helper that downloads a file and returns once finished.",
			Handler:     "BlockingFileDownloadHandler", RequestType: "BlockingFileDownloadTaskData",
		},
		{
			Path: "/wrappers/blocking_request", Methods: []string{"POST"}, Versioned: true, Tag: "wrappers",
			Summary:     "Proxy a blocking HTTP request",
			Description: "Blocking helper that performs an arbitrary HTTP request to the Blendkit server through the Client's configured HTTP client and returns the response.",
			Handler:     "BlockingRequestHandler", RequestType: "BlockingRequestData",
		},
		{
			Path: "/wrappers/nonblocking_request", Methods: []string{"POST"}, Versioned: true, Tag: "wrappers",
			Summary:     "Proxy a non-blocking HTTP request",
			Description: "Schedules an arbitrary HTTP request to the Blendkit server; the response is reported back through /report tasks.",
			Handler:     "NonblockingRequestHandler", RequestType: "NonblockingRequestTaskData",
		},

		// WEB BROWSER (bkclient.js)
		{
			Path: "/bkclientjs/status", Methods: []string{"GET", "OPTIONS"}, Versioned: true, Tag: "bkclientjs",
			Summary:     "Browser: Client status",
			Description: "CORS-enabled endpoint used by bkclient.js in the web browser to read the Client version and the list of connected softwares. Supports OPTIONS preflight.",
			Handler:     "bkclientjsStatusHandler",
		},
		{
			Path: "/bkclientjs/get_asset", Methods: []string{"POST", "OPTIONS"}, Versioned: true, Tag: "bkclientjs",
			Summary:     "Browser: request asset download",
			Description: "CORS-enabled endpoint used by bkclient.js to ask a connected software to download an asset. Supports OPTIONS preflight.",
			Handler:     "bkclientjsGetAssetHandler", RequestType: "bkclientjsDownloadData",
		},

		// GODOT
		{
			Path: "/godot/report", Methods: []string{"POST"}, Versioned: true, Tag: "godot",
			Summary:     "Poll for tasks (Godot)",
			Description: "Polling endpoint for the Godot add-on. Subscribes the app on first call and returns a SoftwareResponse with the Client version, a connection message and pending tasks.",
			Handler:     "godotReportHandler", RequestType: "Software",
		},

		// DEPRECATED - app-prefixed aliases kept for backward compatibility with
		// existing add-ons. New code should use the universal endpoints above.
		{
			Path: "/blender/asset_search", Methods: []string{"POST"}, Versioned: true, Tag: "deprecated",
			Summary:     "Search assets (deprecated)",
			Description: "Deprecated alias of /assets/search. Kept for backward compatibility with existing Blender add-ons.",
			Handler:     "assetSearchHandler", RequestType: "SearchTaskData", Deprecated: true,
		},
		{
			Path: "/blender/asset_download", Methods: []string{"POST"}, Versioned: true, Tag: "deprecated",
			Summary:     "Download an asset (deprecated)",
			Description: "Deprecated alias of /assets/download. Kept for backward compatibility with existing Blender add-ons.",
			Handler:     "assetDownloadHandler", RequestType: "DownloadData", Deprecated: true,
		},
		{
			Path: "/blender/asset_prxc_download", Methods: []string{"POST"}, Versioned: true, Tag: "deprecated",
			Summary:     "Download a proxy collection asset (deprecated)",
			Description: "Deprecated alias of /assets/download_prxc. Kept for backward compatibility with existing Blender add-ons.",
			Handler:     "assetPrxcDownloadHandler", RequestType: "DownloadPrxcData",
			RequestNote: "Body embeds DownloadPrxcData plus an app_id field.", Deprecated: true,
		},
		{
			Path: "/blender/asset_upload", Methods: []string{"POST"}, Versioned: true, Tag: "deprecated",
			Summary:     "Upload an asset (deprecated)",
			Description: "Deprecated alias of /assets/upload. Kept for backward compatibility with existing Blender add-ons.",
			Handler:     "assetUploadHandler", RequestType: "AssetUploadRequestData", RequiresAPIKey: true, Deprecated: true,
		},
		{
			Path: "/blender/cancel_download", Methods: []string{"POST"}, Versioned: true, Tag: "deprecated",
			Summary:     "Cancel a download (deprecated)",
			Description: "Deprecated alias of /assets/cancel_download. Kept for backward compatibility with existing Blender add-ons.",
			Handler:     "CancelDownloadHandler", RequestType: "CancelDownloadData", Deprecated: true,
		},
		{
			Path: "/blender/unsubscribe_addon", Methods: []string{"POST"}, Versioned: true, Tag: "deprecated",
			Summary:     "Unsubscribe a Blender add-on (deprecated)",
			Description: "Deprecated alias of /addons/unsubscribe. Kept for backward compatibility with existing Blender add-ons.",
			Handler:     "blenderUnsubscribeAddonHandler", RequestType: "ReportData", Deprecated: true,
		},
		{
			Path: "/godot/unsubscribe_addon", Methods: []string{"POST"}, Versioned: true, Tag: "deprecated",
			Summary:     "Unsubscribe the Godot add-on (deprecated)",
			Description: "Deprecated alias of /addons/unsubscribe. Kept for backward compatibility with existing Godot add-ons.",
			Handler:     "godotUnsubscribeAddonHandler", RequestType: "ReportData", Deprecated: true,
		},
	}
}
