#!/bin/bash

set -e

# === 参数定义 ===
SOURCE_IMAGE="gcr.io/cadvisor/cadvisor:v0.49.1"
TARGET_REPO="your-own-registry.com/your-project/cadvisor"
TAG="v0.49.1"

echo "🚀 获取 source image manifest 信息..."
INSPECT_JSON=$(docker manifest inspect $SOURCE_IMAGE)

# === 提取 digest ===
DIGEST_AMD64=$(echo "$INSPECT_JSON" | jq -r '.manifests[] | select(.platform.architecture=="amd64") | .digest')
DIGEST_ARM64=$(echo "$INSPECT_JSON" | jq -r '.manifests[] | select(.platform.architecture=="arm64") | .digest')

# 检查是否成功
if [[ -z "$DIGEST_AMD64" || -z "$DIGEST_ARM64" ]]; then
  echo "❌ 没有找到对应架构的 digest，请检查源镜像平台支持情况"
  exit 1
fi

# === 构造完整镜像名 ===
SRC_AMD64="$SOURCE_IMAGE@$DIGEST_AMD64"
SRC_ARM64="$SOURCE_IMAGE@$DIGEST_ARM64"

# === 打 tag 为目标仓库版本 ===
TAG_AMD64="$TARGET_REPO:$TAG-amd64"
TAG_ARM64="$TARGET_REPO:$TAG-arm64"
TAG_MULTI="$TARGET_REPO:$TAG-allarch"

echo "🏷️ 打标签"
docker pull $SRC_AMD64
docker pull $SRC_ARM64
docker tag $SRC_AMD64 $TAG_AMD64
docker tag $SRC_ARM64 $TAG_ARM64

echo "📤 推送各平台镜像"
docker push $TAG_AMD64
docker push $TAG_ARM64

echo "🔗 创建并推送 manifest list"
docker manifest create $TAG_MULTI \
  $TAG_AMD64 \
  $TAG_ARM64

docker manifest push $TAG_MULTI

echo "✅ 构建完成：$TAG_MULTI 已推送"

# 删除本地镜像和manifest
docker rmi $TAG_AMD64 $TAG_ARM64 $TAG_MULTI
docker manifest rm $TAG_MULTI