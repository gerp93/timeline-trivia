// TimelineTrivia Game JavaScript

let timelineTriviaConn = null;

function initTimelineTriviaWebSocket(lobbyId, playerId) {
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

        // Handle result popups (correct/incorrect placement)
        if (messageText.startsWith("result:")) {
            const parts = messageText.split(":");
            // format: result:playerName:correct/incorrect/revealed:message
            const playerName = parts[1];
            const resultType = parts[2]; // "correct", "incorrect", or "revealed" (everyone missed)
            const message = parts.slice(3).join(":");
            showResultPopup(playerName, resultType, message);
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
        })
        .catch(e => console.error("[TimelineTrivia] timeline error:", e));

    // Refresh players list
    htmx.ajax("GET", "/api/timeline-trivia/" + lobbyId + "/players", {
        target: "#players-inline",
        swap: "innerHTML"
    }).then(() => console.log("[TimelineTrivia] players refreshed"));

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

function showAlert(message) {
    // Could use a modal or toast notification
    const messageDiv = document.getElementById("timeline-trivia-message");
    if (messageDiv) {
        messageDiv.textContent = message;
        messageDiv.className = "alert-message";
        setTimeout(() => {
            messageDiv.textContent = "";
        }, 3000);
    }
}

function showResultPopup(playerName, resultType, message) {
    // Remove any existing popup
    const existing = document.querySelector(".timeline-trivia-popup-backdrop");
    if (existing) existing.remove();

    // Create backdrop
    const backdrop = document.createElement("div");
    backdrop.className = "timeline-trivia-popup-backdrop";

    // "revealed" = every player missed the card; it reuses the "incorrect"
    // styling but reveals the actual year and stays up longer since there's
    // more to read.
    const isRevealed = resultType === "revealed";
    const styleClass = isRevealed ? "incorrect" : resultType;

    // Create popup
    const popup = document.createElement("div");
    popup.className = "timeline-trivia-popup " + styleClass;

    const icon = resultType === "correct" ? "✓" : "✗";
    const title = resultType === "correct" ? "CORRECT!" : isRevealed ? "NOBODY GOT IT!" : "WRONG!";

    popup.innerHTML = `
        <span class="popup-icon">${icon}</span>
        ${title}
        <div class="popup-message">${playerName}: ${message}</div>
    `;

    backdrop.appendChild(popup);
    document.body.appendChild(backdrop);

    // Auto-remove; "revealed" gets extra time since it has more to read
    setTimeout(() => {
        backdrop.remove();
    }, isRevealed ? 5000 : 2000);

    // Also allow click to dismiss
    backdrop.addEventListener("click", () => {
        backdrop.remove();
    });
}

// HTMX event handlers for after-swap updates
document.addEventListener("htmx:afterSwap", function(event) {
    // Handle any post-swap logic if needed
});
