// TimelineTrivia Game JavaScript

let timelineTriviaConn = null;

// Per-turn countdown, in seconds; 0 = off. Kept in sync with the lobby setting
// via the "turnTimer:" websocket hint so a change takes effect without a
// reload.
let timelineTriviaTurnTimerSeconds = 0;

// How long the bottom status line stays up. Every client writes every guess
// outcome here, so it needs to last long enough to read but clear before it
// goes stale.
const STATUS_MESSAGE_MS = 8000;
let statusMessageTimeout = null;

// True from the moment a "result:" (correct/incorrect/revealed) popup is
// requested until that popup — and, if the timer is on, the turn-countdown
// popup after it — has finished. While true, restartTurnTimer defers: the
// clock must not start (or keep ticking) behind a full-screen popup that
// covers the board.
let deferTimerStart = false;

function initTimelineTriviaWebSocket(lobbyId, playerId, turnTimerSeconds) {
    timelineTriviaTurnTimerSeconds = parseInt(turnTimerSeconds) || 0;

    let wsProtocol = "wss://";
    if (document.location.protocol === "http:") {
        wsProtocol = "ws://";
    }

    timelineTriviaConn = new WebSocket(wsProtocol + document.location.host + "/ws/lobby/" + lobbyId);

    if (!timelineTriviaConn) {
        alert("Failed to make connection.");
        document.location.href = "/timeline-trivia/lobbies";
        return;
    }

    timelineTriviaConn.onclose = () => {
        alert("Connection Lost");
        document.location.href = "/timeline-trivia/lobbies";
    };

    // Chat form handling
    const chatForm = document.getElementById("timeline-trivia-chat-form");
    const chatMessages = document.getElementById("timeline-trivia-chat-messages");
    const chatInput = document.getElementById("timeline-trivia-chat-input");

    gsChat.wireForm(chatForm, chatInput, timelineTriviaConn);

    // The timeline fragment is what says whose turn it is, and on first paint
    // it arrives via its own hx-trigger="load" rather than through
    // refreshGameState — so start the clock off that swap.
    document.body.addEventListener("htmx:afterSwap", (event) => {
        if (event.detail.target && event.detail.target.id === "timeline-trivia-timeline") {
            restartTurnTimer(lobbyId);
        }
    });

    timelineTriviaConn.onmessage = (event) => {
        let messageText = event.data;
        console.log("[TimelineTrivia WS] Received:", messageText);

        switch (messageText) {
            case "refresh":
                // Refresh all game components
                console.log("[TimelineTrivia WS] Refreshing game state...");
                refreshGameState(lobbyId);
                return;

            case "reload":
                // Game start/reset: refresh game state and controls without a
                // full page navigation. A full location.reload() drops this
                // websocket connection; if this player is the only client,
                // the server deletes the (now empty) lobby before the reload
                // can finish, destroying the game that was just started.
                console.log("[TimelineTrivia WS] Refreshing game in 500ms...");
                setTimeout(() => {
                    refreshGameState(lobbyId);
                    refreshControls(lobbyId);
                }, 500);
                return;

            case "kick":
                document.location.href = "/timeline-trivia/lobbies";
                return;
        }

        // Handle result popups (correct/incorrect placement). The payload is
        // JSON so player names and card text can contain anything, and so the
        // winner's celebration can ride along.
        if (messageText.startsWith("result:")) {
            let payload;
            try {
                payload = JSON.parse(messageText.substring("result:".length));
            } catch (e) {
                console.error("[TimelineTrivia] bad result payload:", e);
                return;
            }
            // Everyone sees what happened, not just the player who acted.
            showStatusMessage(payload.bottomMessage);

            // Freeze the visible clock immediately — it belongs to a turn
            // that just ended, and must not keep ticking (or hit zero)
            // behind the result popup.
            deferTimerStart = true;
            if (typeof gsTimer !== "undefined") gsTimer.stop();

            showResultPopup(payload, () => {
                const next = payload.nextPlayerName;
                if (next && timelineTriviaTurnTimerSeconds > 0) {
                    // Only once this second popup clears does the board
                    // "reopen" and the real timer start.
                    showTurnCountdown(next, () => {
                        deferTimerStart = false;
                        doRestartTurnTimer(lobbyId);
                    });
                } else {
                    deferTimerStart = false;
                    doRestartTurnTimer(lobbyId);
                }
            });
            return;
        }

        // Lightweight status-line-only update, for events that aren't a
        // guess outcome (e.g. Skip & Remove) and so don't get a popup.
        if (messageText.startsWith("status:")) {
            showStatusMessage(messageText.substring("status:".length));
            return;
        }

        // Handle chat messages
        if (messageText.startsWith("chat:")) {
            const chatContent = messageText.substring(5);
            addChatMessage(chatContent);
            return;
        }

        // Handle alert messages
        if (messageText.startsWith("alert:")) {
            const alertContent = messageText.substring(6);
            showAlert(alertContent);
            return;
        }

        // Handle lobby message updates (shown persistently under the lobby name)
        if (messageText.startsWith("lobbyMessage:")) {
            const lobbyMessage = messageText.substring("lobbyMessage:".length);
            updateLobbyMessageBanner(lobbyMessage);
            return;
        }

        // Handle turn timer setting changes (framework lobby setting)
        if (messageText.startsWith("turnTimer:")) {
            timelineTriviaTurnTimerSeconds = parseInt(messageText.substring("turnTimer:".length)) || 0;
            restartTurnTimer(lobbyId);
            return;
        }

        // Default: treat as chat message
        addChatMessage(messageText);
    };
}

function refreshGameState(lobbyId) {
    console.log("[TimelineTrivia] refreshGameState called with lobbyId:", lobbyId);

    // Refresh current card
    htmx.ajax("GET", "/api/timeline-trivia/" + lobbyId + "/current-card", {
        target: "#current-card-content",
        swap: "innerHTML"
    }).then(() => console.log("[TimelineTrivia] current-card refreshed"));

    // Refresh timeline using fetch directly
    const timelineTarget = document.getElementById("timeline-trivia-timeline");
    console.log("[TimelineTrivia] Timeline target element:", timelineTarget);
    const timelineUrl = "/api/timeline-trivia/" + lobbyId + "/timeline?t=" + Date.now();
    console.log("[TimelineTrivia] Fetching timeline from:", timelineUrl);
    fetch(timelineUrl, { cache: 'no-store' })
        .then(response => response.text())
        .then(html => {
            console.log("[TimelineTrivia] Got timeline HTML, length:", html.length);
            if (timelineTarget) {
                timelineTarget.innerHTML = html;
                htmx.process(timelineTarget); // Process HTMX attributes in new content
            }
            console.log("[TimelineTrivia] timeline refreshed");
            // The timeline is what says whose turn it is, so the countdown can
            // only be (re)started once it has landed.
            restartTurnTimer(lobbyId);
        })
        .catch(e => console.error("[TimelineTrivia] timeline error:", e));

    // Refresh the deck breakdown shown in the header tooltip
    htmx.ajax("GET", "/api/timeline-trivia/" + lobbyId + "/decks", {
        target: "#deck-info-tooltip",
        swap: "innerHTML"
    }).catch(e => console.error("[TimelineTrivia] decks error:", e));

    // Refresh draw pile count
    fetch("/api/timeline-trivia/" + lobbyId + "/draw-pile-count", { cache: 'no-store' })
        .then(response => response.text())
        .then(count => {
            const el = document.getElementById("draw-pile-count");
            if (el) {
                el.innerHTML = "Remaining: <strong>" + count + "</strong>";
            }
        })
        .catch(e => console.error("[TimelineTrivia] draw-pile-count error:", e));
}

function refreshControls(lobbyId) {
    // Re-fetches the current page and swaps in just the #timeline-trivia-controls
    // block (Start/Reset button, waiting/winner text) so a game-status change
    // is reflected without a full page navigation.
    fetch(location.pathname, { cache: "no-store" })
        .then(response => response.text())
        .then(html => {
            const doc = new DOMParser().parseFromString(html, "text/html");
            const newControls = doc.getElementById("timeline-trivia-controls");
            const currentControls = document.getElementById("timeline-trivia-controls");
            if (newControls && currentControls) {
                currentControls.outerHTML = newControls.outerHTML;
                htmx.process(document.getElementById("timeline-trivia-controls"));
            }
        })
        .catch(e => console.error("[TimelineTrivia] controls refresh error:", e));
}

// restartTurnTimer is the guarded entry point every refresh path calls. It
// defers to doRestartTurnTimer immediately UNLESS a result/countdown popup
// sequence is in flight, in which case that sequence's own completion
// callback is what eventually starts the clock (see the "result:" handler).
function restartTurnTimer(lobbyId) {
    if (deferTimerStart) return;
    doRestartTurnTimer(lobbyId);
}

// doRestartTurnTimer runs the countdown afresh for the current guesser's
// turn, unconditionally — callers are responsible for the "not while a popup
// is showing" rule above. Everyone watches the same clock, but only the
// guesser's own browser reports the timeout — the server re-checks whose
// turn it is regardless.
function doRestartTurnTimer(lobbyId) {
    const timerEl = document.getElementById("turn-timer");
    if (!timerEl || typeof gsTimer === "undefined") return;

    if (!timelineTriviaTurnTimerSeconds) {
        gsTimer.reset(timerEl, 0);
        return;
    }

    // The timeline fragment marks the row that is both the current guesser's
    // and mine; its presence is how this client knows the clock is on it.
    const isMyTurn = !!document.querySelector(".player-timeline-row.is-current.is-me");

    // No current guesser (game waiting or finished) means no clock to run.
    const hasCurrentGuesser = !!document.querySelector(".player-timeline-row.is-current");
    if (!hasCurrentGuesser) {
        gsTimer.reset(timerEl, 0);
        return;
    }

    gsTimer.start(timerEl, timelineTriviaTurnTimerSeconds, () => {
        if (!isMyTurn) return;
        fetch("/api/timeline-trivia/" + lobbyId + "/timeout", { method: "POST" })
            .catch(e => console.error("[TimelineTrivia] timeout error:", e));
    });
}

// showTurnCountdown plays a 3-2-1 countdown naming who's up next, then calls
// onDone. Shown between the result popup clearing and the real timer
// starting, so nobody sees the board "reopen" until the clock is genuinely
// about to run. Only called when a turn timer is configured — see the
// "result:" handler, which skips straight to onDone when it's off.
function showTurnCountdown(nextPlayerName, onDone) {
    const existing = document.querySelector(".timeline-trivia-popup-backdrop");
    if (existing) existing.remove();

    const backdrop = document.createElement("div");
    backdrop.className = "timeline-trivia-popup-backdrop";

    const popup = document.createElement("div");
    popup.className = "timeline-trivia-popup turn-countdown";

    const numberEl = document.createElement("div");
    numberEl.className = "countdown-number";
    popup.appendChild(numberEl);

    const nameEl = document.createElement("div");
    nameEl.className = "countdown-name";
    nameEl.textContent = nextPlayerName + "'s turn";
    popup.appendChild(nameEl);

    backdrop.appendChild(popup);
    document.body.appendChild(backdrop);

    let finished = false;
    let count = 3;
    numberEl.textContent = count;

    function finish() {
        if (finished) return;
        finished = true;
        clearInterval(interval);
        backdrop.remove();
        if (typeof onDone === "function") onDone();
    }

    const interval = setInterval(() => {
        count -= 1;
        if (count > 0) {
            numberEl.textContent = count;
        } else {
            finish();
        }
    }, 1000);

    // Click to skip the countdown, same as the result popup allows.
    backdrop.addEventListener("click", finish);
}

function addChatMessage(message) {
    // Shared renderer: parses <blue>/<green>/<red>/</> color tokens, timestamps,
    // and trims history (see gameshell-framework /gs/js/chat.js).
    gsChat.append(document.getElementById("timeline-trivia-chat-messages"), message);
}

function updateLobbyMessageBanner(message) {
    const banner = document.getElementById("lobby-message-banner");
    if (!banner) return;
    banner.textContent = message;
    banner.style.display = message ? "" : "none";
}

// showStatusMessage writes the bottom-of-screen status line. Driven by the
// websocket rather than by the acting player's own HTTP response, so every
// player sees every outcome.
function showStatusMessage(message) {
    if (!message) return;
    const messageDiv = document.getElementById("timeline-trivia-message");
    if (!messageDiv) return;

    messageDiv.textContent = message;
    if (statusMessageTimeout) clearTimeout(statusMessageTimeout);
    statusMessageTimeout = setTimeout(() => {
        messageDiv.textContent = "";
    }, STATUS_MESSAGE_MS);
}

function showAlert(message) {
    showStatusMessage(message);
}

// showResultPopup renders the guess-outcome modal. On a correct guess it also
// shows the winner's personal celebration GIF/message if they set one on their
// account page. onDone fires exactly once, whether the popup auto-dismisses
// or is clicked away.
function showResultPopup(payload, onDone) {
    // Remove any existing popup
    const existing = document.querySelector(".timeline-trivia-popup-backdrop");
    if (existing) existing.remove();

    // Create backdrop
    const backdrop = document.createElement("div");
    backdrop.className = "timeline-trivia-popup-backdrop";

    // "revealed" = every player missed the card; it reuses the "incorrect"
    // styling but reveals the actual year and stays up longer since there's
    // more to read.
    const isRevealed = payload.type === "revealed";
    const styleClass = isRevealed ? "incorrect" : payload.type;

    // Create popup
    const popup = document.createElement("div");
    popup.className = "timeline-trivia-popup " + styleClass;

    const icon = payload.type === "correct" ? "✓" : "✗";
    const title = payload.type === "correct" ? "CORRECT!" : isRevealed ? "NOBODY GOT IT!" : "WRONG!";

    // Built node-by-node rather than with innerHTML: player names and card
    // text are user-authored and must not be parsed as markup.
    const iconEl = document.createElement("span");
    iconEl.className = "popup-icon";
    iconEl.textContent = icon;
    popup.appendChild(iconEl);
    popup.appendChild(document.createTextNode(title));

    const messageEl = document.createElement("div");
    messageEl.className = "popup-message";
    messageEl.textContent = payload.playerName + ": " + payload.message;
    popup.appendChild(messageEl);

    const hasCelebration = payload.type === "correct" && (payload.hasGif || payload.celebration);
    if (payload.hasGif && payload.userId) {
        const gif = document.createElement("img");
        gif.className = "popup-gif";
        gif.alt = "";
        gif.src = "/api/user/" + encodeURIComponent(payload.userId) + "/win-gif";
        popup.appendChild(gif);
    }
    if (payload.type === "correct" && payload.celebration) {
        const celebrationEl = document.createElement("div");
        celebrationEl.className = "popup-celebration-message";
        celebrationEl.textContent = payload.celebration;
        popup.appendChild(celebrationEl);
    }

    backdrop.appendChild(popup);
    document.body.appendChild(backdrop);

    let finished = false;
    function finish() {
        if (finished) return;
        finished = true;
        clearTimeout(dismissTimer);
        backdrop.remove();
        if (typeof onDone === "function") onDone();
    }

    // Auto-remove; "revealed" and celebrations get extra time to read
    let dismissAfter = 2000;
    if (isRevealed) dismissAfter = 5000;
    if (hasCelebration) dismissAfter = 5000;
    const dismissTimer = setTimeout(finish, dismissAfter);

    // Also allow click to dismiss
    backdrop.addEventListener("click", finish);
}
