.PHONY: up up-lab down logs ps rebuild

COMPOSE_LAB = docker compose -f docker-compose.yml -f docker-compose.lab.yml -f docker-compose.override.yml

# 既存の最小構成 (mysql + backend のみ)
up:
	docker compose up -d

# lab トピック用の追加サービス込み (LocalStack, Postgres, Redis)
up-lab:
	$(COMPOSE_LAB) up -d

down:
	$(COMPOSE_LAB) down

logs:
	$(COMPOSE_LAB) logs -f --tail 100

ps:
	$(COMPOSE_LAB) ps

rebuild:
	$(COMPOSE_LAB) up -d --build
