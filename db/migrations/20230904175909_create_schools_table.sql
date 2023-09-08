-- +goose Up
-- +goose StatementBegin

CREATE TABLE `schools` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `is_show` tinyint(1) DEFAULT NULL,
  `name` longtext,
  `monthly_fee` bigint DEFAULT NULL,
  `term_num` bigint DEFAULT NULL,
  `term_unit` bigint DEFAULT NULL,
  `remarks` longtext,
  `overview` longtext,
  `image_links` json DEFAULT NULL,
  `link` longtext,
  `recommendations` json DEFAULT NULL,
  `features` json DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_schools_deleted_at` (`deleted_at`)
) ENGINE=InnoDB AUTO_INCREMENT=9 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE `schools`;

-- +goose StatementEnd
