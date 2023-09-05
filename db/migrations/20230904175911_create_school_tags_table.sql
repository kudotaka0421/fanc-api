-- +goose Up
-- +goose StatementBegin

CREATE TABLE `school_tags` (
  `school_id` int unsigned NOT NULL,
  `tag_id` int unsigned NOT NULL,
  PRIMARY KEY (`school_id`,`tag_id`),
  KEY `fk_school_tags_tag` (`tag_id`),
  CONSTRAINT `fk_school_tags_school` FOREIGN KEY (`school_id`) REFERENCES `schools` (`id`),
  CONSTRAINT `fk_school_tags_tag` FOREIGN KEY (`tag_id`) REFERENCES `tags` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE `school_tags`;

-- +goose StatementEnd
