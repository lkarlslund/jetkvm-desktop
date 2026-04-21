# openh264-go

OpenH264の静的ライブラリバインディングを提供するGoパッケージです。

[pion/mediadevices](https://github.com/pion/mediadevices)のOpenH264実装をベースに、静的ライブラリを同梱した独立パッケージとして切り出しました。

## 特徴

- **静的リンク**: OpenH264の静的ライブラリ(.a)を同梱しているため、システムにOpenH264をインストールする必要がありません。`go build`するだけで、OpenH264が実行バイナリに組み込まれます。
- **クロスプラットフォーム**: Linux (amd64/arm64/armv7)、macOS (amd64/arm64)、Windows (amd64)をサポート
- **シンプルなAPI**: エンコーダーとデコーダーを独立した構造体として提供
- **外部依存なし**: 純粋なGoとCGOのみで動作

## インストール

```bash
go get github.com/Azunyan1111/openh264-go
```

## 使用例

### エンコード

```go
package main

import (
    "image"
    openh264 "github.com/Azunyan1111/openh264-go"
)

func main() {
    // エンコーダーの作成
    params := openh264.NewEncoderParams()
    params.Width = 640
    params.Height = 480
    params.BitRate = 500000

    encoder, err := openh264.NewEncoder(params)
    if err != nil {
        panic(err)
    }
    defer encoder.Close()

    // YCbCr画像をエンコード
    img := image.NewYCbCr(
        image.Rect(0, 0, 640, 480),
        image.YCbCrSubsampleRatio420,
    )
    // ... imgにデータを設定 ...

    h264Data, err := encoder.Encode(img)
    if err != nil {
        panic(err)
    }
    // h264Dataを使用
}
```

### デコード

```go
package main

import (
    "io"
    openh264 "github.com/Azunyan1111/openh264-go"
)

func main() {
    // io.ReaderからH.264ストリームを読み込む場合
    var h264Stream io.Reader // = ...

    decoder, err := openh264.NewDecoder(h264Stream)
    if err != nil {
        panic(err)
    }
    defer decoder.Close()

    for {
        img, err := decoder.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            panic(err)
        }
        // imgを使用 (*image.YCbCr)
    }
}
```

### 生データを直接デコード

```go
// Decodeメソッドで生のH.264データを直接デコード
img, err := decoder.Decode(h264Data)
if err != nil {
    panic(err)
}
if img != nil {
    // フレームが利用可能
}

// ストリーム終了時にバッファに残ったフレームを取得
for {
    img, err := decoder.Flush()
    if err != nil || img == nil {
        break
    }
    // 残りのフレームを処理
}
```

## 動的リンク

システムにインストールされたOpenH264を使用する場合は、`dynamic`ビルドタグを指定します：

```bash
go build -tags dynamic
```

この場合、pkg-configでOpenH264が見つかる必要があります。

## ライセンス

- Goバインディングコード: MIT License (based on [pion/mediadevices](https://github.com/pion/mediadevices))
- OpenH264静的ライブラリ (`lib/`): BSD-2-Clause (Cisco Systems) - [lib/LICENSE](lib/LICENSE)

## 謝辞

- [pion/mediadevices](https://github.com/pion/mediadevices) - 本パッケージのベースとなった実装
- [Cisco OpenH264](https://github.com/cisco/openh264) - H.264コーデックライブラリ
