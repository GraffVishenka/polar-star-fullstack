# --- Сборка ---------------------------------------------------------------
FROM node:22-alpine AS build
WORKDIR /app

# Точная версия pnpm и конфиг workspace (onlyBuiltDependencies/strictDepBuilds)
# нужны до install: иначе сборка в Docker ведёт себя иначе, чем локально.
RUN npm install --global pnpm@11.28.5

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .
# tsc -b + vite build; MSW-моки в прод-сборку не попадают (VITE_ENABLE_MSW=false).
ENV VITE_ENABLE_MSW=false
RUN pnpm build

# --- Раздача --------------------------------------------------------------
FROM nginx:1.27-alpine AS runtime

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --quiet --spider http://127.0.0.1/ || exit 1

CMD ["nginx", "-g", "daemon off;"]
