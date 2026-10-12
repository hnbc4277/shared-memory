FROM golang:1.25-alpine AS build
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB="sum.golang.org https://goproxy.cn/sumdb/sum.golang.org"
ENV GOPROXY=${GOPROXY} GOSUMDB=${GOSUMDB} CGO_ENABLED=0 GOTOOLCHAIN=local
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY sdk ./sdk
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && go build -trimpath -ldflags="-s -w" -o /out/mcp ./cmd/mcp

FROM alpine:3.20
ARG APK_MIRROR=mirrors.aliyun.com
# tesseract (image OCR) and poppler (pdftotext) back attachment text extraction.
RUN sed -i "s/dl-cdn.alpinelinux.org/${APK_MIRROR}/g" /etc/apk/repositories \
 && apk add --no-cache tesseract-ocr tesseract-ocr-data-chi_sim tesseract-ocr-data-eng poppler-utils \
 && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/ /usr/local/bin/
COPY migrations ./migrations
USER app
ENV SHARED_MEMORY_ENV=/app/compose.env
EXPOSE 18080
CMD ["api"]
