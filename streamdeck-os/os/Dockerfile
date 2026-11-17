# Build environment for the ISO (Ubuntu 24.04, same release as the kernel used).
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends \
      grub-common grub-efi-amd64-bin grub-pc-bin xorriso mtools kmod zstd cpio \
      gcc-13 make patch binutils libelf1t64 \
      wpasupplicant hostapd ca-certificates python3 git curl \
 && rm -rf /var/lib/apt/lists/* && apt-get update
RUN curl -fsSL https://go.dev/dl/go1.23.12.linux-amd64.tar.gz | tar -C /usr/local -xz
ENV PATH=/usr/local/go/bin:$PATH
