module kyoku.example/hello

go 1.25.6

require github.com/zertex-labs/kyoku-hub/packages/plugin-sdk v0.0.0

require github.com/zertex-labs/kyoku-hub/packages/plugin-api v0.0.0 // indirect

replace github.com/zertex-labs/kyoku-hub/packages/plugin-api => ../../../packages/plugin-api

replace github.com/zertex-labs/kyoku-hub/packages/plugin-sdk => ../../../packages/plugin-sdk
