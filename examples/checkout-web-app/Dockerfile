FROM golang:1.22-alpine AS build

WORKDIR /src
COPY . .
WORKDIR /src/examples/checkout-web-app
RUN CGO_ENABLED=0 go build -o /out/webirr-checkout-web-app .

FROM alpine:3.20

RUN addgroup -S webirr && adduser -S webirr -G webirr
WORKDIR /app
COPY --from=build /out/webirr-checkout-web-app /app/webirr-checkout-web-app
COPY --from=build /src/examples/checkout-web-app/assets /app/assets
COPY --from=build /src/examples/checkout-web-app/static /app/static
COPY --from=build /src/examples/checkout-web-app/templates /app/templates
RUN mkdir -p /data && chown -R webirr:webirr /app /data

USER webirr
EXPOSE 8080
CMD ["/app/webirr-checkout-web-app"]
