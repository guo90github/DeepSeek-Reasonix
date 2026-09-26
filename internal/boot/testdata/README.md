# room_wake_payload.json

Frozen producer payload: the `params` a chatting MCP child sends as
`notifications/chatting/room_message` (chatting `cmd/chatting/mcp.go`,
`notifyRoomMessage`: `seq` / `from` / `text` / `topic` / `mentions` / `kind` /
`origin` / `panel`, rules B108 + B119).

The producing repository owns these bytes: it keeps them at
`cmd/chatting/testdata/room_wake_payload.json` (md5
`b5340fd038f96b1b0a5d3464c4f7c7c4`, 247 bytes) and is the only side allowed to
change them. Its canonical form is `json.MarshalIndent(params, "", "  ")` plus a
trailing newline, so the file is one serialization of the producer's own params,
never a hand-written sample. `TestRoomWakeFixtureMatchesTheProducerCopy` compares
the two copies byte for byte whenever `CHATTING_REPO` names the chatting
checkout, and prints that it did not run when the variable is unset — a payload
change made on one side must turn that test red, never drift quietly.

Refreshing: change the payload on the producing side first (`CHATTING_UPDATE_FIXTURE=1
go test -run TestPushProducerPayloadMatchesFrozenFixture ./cmd/chatting`), copy
the bytes here unchanged, then run `go test ./internal/boot/ -run RoomWake`.
