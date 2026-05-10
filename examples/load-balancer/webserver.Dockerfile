FROM nginx:alpine

# Generate index.html at container startup using POD_NAME
RUN printf '%s\n' \
'#!/bin/sh' \
'echo "Hello from ${POD_NAME}!" > /usr/share/nginx/html/index.html' \
'exec nginx -g "daemon off;"' \
> /docker-entrypoint-custom.sh \
&& chmod +x /docker-entrypoint-custom.sh

CMD ["/docker-entrypoint-custom.sh"]
