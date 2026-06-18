.PHONY: setup dev check reset-local

setup:
	cd backend && go mod download
	cd frontend && npm ci

dev:
	bash scripts/dev.sh

check:
	bash scripts/check.sh

reset-local:
	@printf "这会删除 .local/ 下的本地测试用户、Cookie 和设备状态，继续吗？[y/N] "; \
	read answer; \
	if [ "$$answer" = "y" ] || [ "$$answer" = "Y" ]; then \
		rm -rf .local; \
		echo "本地测试状态已删除。下次 make dev 会重新创建 admin。"; \
	else \
		echo "已取消。"; \
	fi
