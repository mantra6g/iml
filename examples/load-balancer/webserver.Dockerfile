FROM nginx:alpine

# Configure nginx to listen on IPv4 + IPv6 on all interfaces
RUN printf '%s\n' \
'server {' \
'    listen 80;' \
'    listen [::]:80;' \
'' \
'    location / {' \
'        root /usr/share/nginx/html;' \
'        index index.html;' \
'    }' \
'}' \
> /etc/nginx/conf.d/default.conf

# Generate index.html at container startup using POD_NAME
RUN printf '%s\n' \
'#!/bin/sh' \
'echo "Hello from ${POD_NAME}!" > /usr/share/nginx/html/index.html' \
'exec nginx -g "daemon off;"' \
> /docker-entrypoint-custom.sh \
&& chmod +x /docker-entrypoint-custom.sh

CMD ["/docker-entrypoint-custom.sh"]
