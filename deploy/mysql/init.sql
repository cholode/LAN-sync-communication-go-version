-- ============================================================================
-- 生产环境初始化脚本：Lan IM 即时通讯系统
-- 存储引擎：InnoDB
-- 字符集：utf8mb4（支持 Emoji/生僻字）
-- 设计规范：禁用物理外键，采用逻辑外键保证高并发性能；软删除+唯一索引约束
-- ============================================================================

CREATE DATABASE IF NOT EXISTS `lan_im` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE `lan_im`;

-- ----------------------------------------------------------------------------
-- 表结构：用户基础信息表
-- 业务说明：存储系统所有用户的核心身份信息
-- ----------------------------------------------------------------------------
CREATE TABLE `users` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '用户唯一ID',
  `username`    VARCHAR(64) NOT NULL COMMENT '登录用户名',
  `password`    VARCHAR(255) NOT NULL COMMENT 'Bcrypt加密密码',
  `avatar`      VARCHAR(255) DEFAULT '' COMMENT '用户头像地址',
  `role`        TINYINT NOT NULL DEFAULT 0 COMMENT '用户角色：0-普通用户 1-超级管理员',
  `created_at`  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at`  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at`  DATETIME(3) DEFAULT NULL COMMENT '软删除时间',
  PRIMARY KEY (`id`),
  -- 唯一约束：用户名+软删除标记，解决软删除场景下的用户名唯一性问题
  UNIQUE KEY `idx_username_deleted` (`username`, `deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户基础信息表';

-- ----------------------------------------------------------------------------
-- 表结构：会话房间表
-- 业务说明：统一管理单聊/群聊房间，支持软删除
-- ----------------------------------------------------------------------------
CREATE TABLE `rooms` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '房间唯一ID',
  `type`        TINYINT NOT NULL COMMENT '房间类型：1-单聊 2-群聊',
  `name`        VARCHAR(128) DEFAULT '' COMMENT '房间名称（群聊必填，单聊可为空）',
  `creator_id`  BIGINT UNSIGNED NOT NULL COMMENT '创建者用户ID',
  `created_at`  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at`  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at`  DATETIME(3) DEFAULT NULL COMMENT '软删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='会话房间表（单聊/群聊统一存储）';

-- ----------------------------------------------------------------------------
-- 表结构：房间成员关联表
-- 业务说明：维护用户与房间的关联关系，替代传统好友/群成员表
-- ----------------------------------------------------------------------------
CREATE TABLE `room_members` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `room_id`     BIGINT UNSIGNED NOT NULL COMMENT '房间ID',
  `user_id`     BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
  `role`        TINYINT NOT NULL DEFAULT 1 COMMENT '成员角色：1-普通成员 2-管理员 3-群主',
  `joined_at`   DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '加入时间',
  PRIMARY KEY (`id`),
  -- 唯一约束：禁止同一用户重复加入同一房间
  UNIQUE KEY `idx_room_user` (`room_id`, `user_id`),
  -- 索引：优化用户所属房间列表查询性能
  KEY `idx_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='房间成员关联表';

-- ----------------------------------------------------------------------------
-- 表结构：消息记录表（系统核心表）
-- 业务说明：统一存储所有会话消息，支持大文本/文件消息，高性能分页查询
-- ----------------------------------------------------------------------------
CREATE TABLE `messages` (
  `id`          BIGINT NOT NULL COMMENT '雪花算法生成的消息ID（禁用自增）',
  `room_id`     BIGINT UNSIGNED NOT NULL COMMENT '所属房间ID',
  `sender_id`   BIGINT UNSIGNED NOT NULL COMMENT '发送者用户ID',
  `type`        TINYINT NOT NULL DEFAULT 1 COMMENT '消息类型：1-文本 2-文件/图片 3-系统通知',
  `content`     TEXT NOT NULL COMMENT '消息内容/文件元数据JSON',
  `created_at`  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '消息发送时间',
  `deleted_at`  DATETIME(3) DEFAULT NULL COMMENT '软删除时间',
  PRIMARY KEY (`id`),
  -- 核心索引：优化房间消息分页查询，消除 filesort，保证查询性能
  KEY `idx_room_id_id` (`room_id`, `id` DESC),
  KEY `idx_created_at` (`created_at`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='消息记录表';