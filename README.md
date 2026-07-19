# Timeline Trivia

Version: 0.1.0

Timeline Trivia is a fork of [Card Judge](https://github.com/GrantFBarnes/card-judge)
by [GrantFBarnes](https://github.com/GrantFBarnes), repurposed into a
history-trivia game where players place event cards into a chronological
timeline instead of judging prompt/response cards. Credit and thanks to the
original project for the platform this was built on.

## Environment Variables

```
// MySQL/MariaDB
CARD_TIMELINE_SQL_HOST // ip address of server
CARD_TIMELINE_SQL_DATABASE // database name
CARD_TIMELINE_SQL_USER // database username
CARD_TIMELINE_SQL_PASSWORD // database username password

// Port
CARD_TIMELINE_PORT // [optional] port to serve (defaults to 2016)

// Redirect Logs
CARD_TIMELINE_LOG_FILE // [optional] path to log file (defaults to stdout)

// HTTPS Certificates
CARD_TIMELINE_CERT_FILE // [optional] path to cert file
CARD_TIMELINE_KEY_FILE // [optional] path to key file
```
