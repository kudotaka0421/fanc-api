-- +goose Up
-- +goose StatementBegin
CREATE TABLE `counselings` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `counselee_name` varchar(255) NOT NULL,
  `email` varchar(255) NOT NULL,
  `status` int NOT NULL,
  `date` datetime(3) NOT NULL,
  `remarks` text,
  `message` text,
  `user_id` bigint unsigned,
  PRIMARY KEY (`id`),
  FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE `counseling_schools` (
  `counseling_id` bigint unsigned NOT NULL,
  `school_id` int unsigned NOT NULL,
  PRIMARY KEY (`counseling_id`, `school_id`),
  FOREIGN KEY (`counseling_id`) REFERENCES `counselings`(`id`) ON DELETE CASCADE,
  FOREIGN KEY (`school_id`) REFERENCES `schools`(`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE `counseling_schools`;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE `counselings`;
-- +goose StatementEnd
