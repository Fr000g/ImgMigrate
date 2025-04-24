#!/bin/bash
set -e

# 用法说明
usage() {
  echo "用法: $0 <镜像名称> <操作系统> <架构> <私有仓库地址>(可选)"
  echo "示例: $0 cadvisor:v0.49.1 linux amd64 registry.example.com"
  echo "如果不指定私有仓库地址，只会进行拉取并添加架构标签"
  exit 1
}

# 检查参数
if [ "$#" -lt 3 ]; then
  usage
fi

# 获取参数
IMAGE_NAME="$1"
TARGET_OS="$2"
TARGET_ARCH="$3"
PRIVATE_REGISTRY="${4:-}"

# 检查Docker是否可用
if ! command -v docker &> /dev/null; then
  echo "错误: 未找到docker命令，请安装Docker"
  exit 1
fi

echo "=== 镜像信息 ==="
echo "原始镜像: $IMAGE_NAME"
echo "目标OS: $TARGET_OS"
echo "目标架构: $TARGET_ARCH"
if [ -n "$PRIVATE_REGISTRY" ]; then
  echo "私有仓库: $PRIVATE_REGISTRY"
fi
echo "================="

# 从镜像名称中提取基本名称和标签
if [[ "$IMAGE_NAME" == *":"* ]]; then
  BASE_NAME=$(echo "$IMAGE_NAME" | cut -d ':' -f 1)
  TAG=$(echo "$IMAGE_NAME" | cut -d ':' -f 2)
else
  BASE_NAME="$IMAGE_NAME"
  TAG="latest"
fi

echo "基本名称: $BASE_NAME"
echo "标签: $TAG"

# 使用docker manifest查找对应digest
echo "正在查询manifest信息..."
# 确保我们有最新的manifest信息
docker manifest inspect "$IMAGE_NAME" > /dev/null 2>&1 || {
  echo "无法获取镜像 $IMAGE_NAME 的manifest信息，尝试拉取..."
  docker pull --platform "$TARGET_OS/$TARGET_ARCH" "$IMAGE_NAME" > /dev/null
  echo "拉取完成，再次尝试获取manifest..."
  docker manifest inspect "$IMAGE_NAME" > /dev/null 2>&1 || {
    echo "错误: 无法获取镜像 $IMAGE_NAME 的manifest信息"
    exit 1
  }
}

# 尝试获取指定OS和架构的digest
DIGEST=$(docker manifest inspect "$IMAGE_NAME" | jq -r --arg OS "$TARGET_OS" --arg ARCH "$TARGET_ARCH" \
  '.manifests[] | select(.platform.os == $OS and .platform.architecture == $ARCH) | .digest')

# 检查digest是否为空
if [ -z "$DIGEST" ]; then
  echo "错误: 未找到 $TARGET_OS/$TARGET_ARCH 对应的digest"
  echo "可用的平台列表:"
  docker manifest inspect "$IMAGE_NAME" | jq -r '.manifests[] | "OS: \(.platform.os), Architecture: \(.platform.architecture), Variant: \(.platform.variant // "none"), Digest: \(.digest)"'
  exit 1
fi

echo "找到 $TARGET_OS/$TARGET_ARCH 对应的digest: $DIGEST"

# 通过digest拉取镜像
echo "正在通过digest拉取镜像..."
DIGEST_IMAGE="$BASE_NAME@$DIGEST"
docker pull "$DIGEST_IMAGE"

# 创建新的标签名称
NEW_TAG="${TAG}-${TARGET_ARCH}"

# 如果提供了私有仓库地址，创建带有私有仓库的标签
if [ -n "$PRIVATE_REGISTRY" ]; then
  # 移除私有仓库地址末尾的斜杠（如果有）
  PRIVATE_REGISTRY="${PRIVATE_REGISTRY%/}"
  
  # 创建新的标签（带有私有仓库地址）
  PRIVATE_TAG="$PRIVATE_REGISTRY/$BASE_NAME:$NEW_TAG"
  echo "重新打标签: $DIGEST_IMAGE -> $PRIVATE_TAG"
  docker tag "$DIGEST_IMAGE" "$PRIVATE_TAG"
  
  # 询问是否推送到私有仓库
  read -p "是否推送镜像到私有仓库 $PRIVATE_REGISTRY? (y/n): " PUSH_CONFIRM
  if [[ "$PUSH_CONFIRM" == "y" || "$PUSH_CONFIRM" == "Y" ]]; then
    echo "推送镜像到私有仓库..."
    docker push "$PRIVATE_TAG"
    echo "推送完成!"
  else
    echo "跳过推送到私有仓库"
  fi
else
  # 创建新的标签（只有架构后缀）
  LOCAL_TAG="$BASE_NAME:$NEW_TAG"
  echo "重新打标签: $DIGEST_IMAGE -> $LOCAL_TAG"
  docker tag "$DIGEST_IMAGE" "$LOCAL_TAG"
fi

echo "操作完成!"
