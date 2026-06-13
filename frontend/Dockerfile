# ---- Build stage ----
FROM oven/bun:1-alpine AS builder

WORKDIR /app
COPY package.json bun.lock ./
RUN bun install


COPY . .

# Ensure Vite loads .env.production (VITE_API_BASE=/api/v1)
# instead of .env.development (http://localhost:3000/api/v1)
ENV NODE_ENV=production
RUN bun run build

# ---- Runtime stage ----
FROM nginx:alpine

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
