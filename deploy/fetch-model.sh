#!/bin/sh
# Downloads the embedding model into /data/$MODEL_DIR once. Files are fetched
# with curl because TEI's own downloader stalls on hf-mirror redirects. The TEI
# CPU image runs ONNX, so the weights come from the ONNX export of the model.
set -eu
MODEL_REPO="${MODEL_REPO:-BAAI/bge-small-zh-v1.5}"
MODEL_REV="${MODEL_REV:-7999e1d3359715c523056ef9478215996d62a620}"
ONNX_REPO="${ONNX_REPO:-Xenova/bge-small-zh-v1.5}"
ONNX_REV="${ONNX_REV:-75c43b069aac4d136ba6bc1122f995fedcfd2781}"
MODEL_DIR="/data/${MODEL_DIR:-bge-small-zh-v1.5}"
HF_ENDPOINT="${HF_ENDPOINT:-https://hf-mirror.com}"
FILES="config.json config_sentence_transformers.json modules.json sentence_bert_config.json special_tokens_map.json tokenizer.json tokenizer_config.json vocab.txt 1_Pooling/config.json"

if [ -f "$MODEL_DIR/.complete" ] && [ -f "$MODEL_DIR/onnx/model.onnx" ]; then
  echo "model present: $MODEL_DIR"
  exit 0
fi
command -v curl >/dev/null 2>&1 || {
  sed -i "s#dl-cdn.alpinelinux.org#${APK_MIRROR:-mirrors.aliyun.com}#g" /etc/apk/repositories
  apk add -q curl
}
fetch() { # repo rev file
  curl -fsSL --retry 5 -o "$MODEL_DIR/$3.part" "$HF_ENDPOINT/$1/resolve/$2/$3"
  mv "$MODEL_DIR/$3.part" "$MODEL_DIR/$3"
}
mkdir -p "$MODEL_DIR/1_Pooling" "$MODEL_DIR/onnx"
for f in $FILES; do
  fetch "$MODEL_REPO" "$MODEL_REV" "$f"
done
fetch "$ONNX_REPO" "$ONNX_REV" onnx/model.onnx
touch "$MODEL_DIR/.complete"
echo "model downloaded: $MODEL_DIR"
