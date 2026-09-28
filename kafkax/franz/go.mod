module github.com/rithulkamesh/toolkit/kafkax/franz

go 1.26.0

replace github.com/rithulkamesh/toolkit/kafkax => ../

require (
	github.com/rithulkamesh/toolkit/kafkax v0.0.0-00010101000000-000000000000
	github.com/twmb/franz-go v1.22.0
)

require (
	github.com/klauspost/compress v1.20.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.30 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.14.0 // indirect
)
