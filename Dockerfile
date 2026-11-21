FROM node:22-bookworm-slim

ENV NODE_ENV=production \
    PORT=8088 \
    DATA_DIR=/data \
    DOWNLOAD_DIR=/downloads

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates libstdc++6 libgcc-s1 libcurl4 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY package.json ./
RUN npm install --omit=dev --no-audit --no-fund
COPY src ./src
COPY public ./public

RUN mkdir -p /data/sdk /downloads \
    && chown -R node:node /app /data /downloads
USER node

EXPOSE 8088
VOLUME ["/data", "/downloads"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD node -e "fetch('http://127.0.0.1:8088/api/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"

CMD ["node", "src/server.js"]
