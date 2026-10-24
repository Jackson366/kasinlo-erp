FROM node:22-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM maven:3.9-eclipse-temurin-21 AS backend
WORKDIR /src/backend-java
COPY backend-java/pom.xml ./
RUN mvn -B dependency:go-offline
COPY backend-java/src ./src
COPY --from=frontend /src/frontend/dist ./src/main/resources/static
RUN mvn -B package

FROM eclipse-temurin:21-jre-jammy
WORKDIR /app
RUN groupadd --system erp && useradd --system --gid erp erp && mkdir -p /app/data && chown erp:erp /app/data
COPY --from=backend /src/backend-java/target/kasinlo-erp-2.0.0-java-SNAPSHOT.jar /app/erp.jar
USER erp
EXPOSE 6085
ENV ERP_HOST=0.0.0.0
ENTRYPOINT ["java","-jar","/app/erp.jar"]
