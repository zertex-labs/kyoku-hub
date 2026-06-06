FROM golang:1.25-alpine AS builder

WORKDIR /src

# Copy only the modules we need (fs + proto/gen)
COPY apps/fs/go.mod apps/fs/go.sum apps/fs/
COPY proto/gen/go.mod proto/gen/go.sum proto/gen/

# Disable workspace mode — we only have fs + proto/gen in the container
ENV GOWORK=off

# Download deps
RUN cd apps/fs && go mod download

# Copy source
COPY apps/fs/ apps/fs/
COPY proto/gen/ proto/gen/

# Build
RUN cd apps/fs && CGO_ENABLED=0 go build -o /bin/fs ./cmd/server/

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /bin/fs /bin/fs

EXPOSE 8000

CMD ["/bin/fs"]
