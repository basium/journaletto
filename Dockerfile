FROM golang:1.26-alpine AS deps
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

FROM deps AS build
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build -trimpath -o /out/journaletto ./cmd/journaletto

FROM alpine:3.21 AS runtime
RUN addgroup -S app && adduser -S -G app app && mkdir /data && chown app:app /data
COPY --from=build /out/journaletto /usr/local/bin/journaletto
ARG VERSION=dev
ARG COMMIT=unknown
ENV PORT=3000 DATA_ROOT=/data DB_PATH=/data/journaletto.db VERSION=${VERSION} COMMIT=${COMMIT}
USER app
VOLUME /data
EXPOSE 3000
HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=5 \
  CMD ["journaletto", "-healthcheck"]
ENTRYPOINT ["journaletto"]
