Содание handlerа с настройками:

`webWriter := axweblog.NewWebLogWriter("/log/", "admin", "admin")`

Подключение в zerolog

`log.Logger = zerolog.New(zerolog.MultiLevelWriter(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05,000"}, webWriter)).
Level(zerolog.DebugLevel).
With().Timestamp().Logger()`

Подключение в http

`r.Handle("/log/*", webWriter)`