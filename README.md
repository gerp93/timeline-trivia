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
TIMELINE_TRIVIA_SQL_HOST // ip address of server
TIMELINE_TRIVIA_SQL_DATABASE // database name
TIMELINE_TRIVIA_SQL_USER // database username
TIMELINE_TRIVIA_SQL_PASSWORD // database username password

// Port
TIMELINE_TRIVIA_PORT // [optional] port to serve (defaults to 2016)

// Redirect Logs
TIMELINE_TRIVIA_LOG_FILE // [optional] path to log file (defaults to stdout)

// HTTPS Certificates
TIMELINE_TRIVIA_CERT_FILE // [optional] path to cert file
TIMELINE_TRIVIA_KEY_FILE // [optional] path to key file
```
