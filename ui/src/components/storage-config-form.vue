<template>
    <div class="storage-config-form">
        <a-form :model="modelValue" validate-trigger="blur" auto-label-width>
            <a-form-item label="access_key">
                <a-input
                    :model-value="modelValue.access_key"
                    :disabled="disabled"
                    :spellcheck="false"
                    placeholder="请输入"
                    @update:model-value="updateField('access_key', $event)"
                />
                <template #extra>s3 access_key</template>
            </a-form-item>
            <a-form-item label="secret_key">
                <a-input
                    :model-value="modelValue.secret_key"
                    :disabled="disabled"
                    :spellcheck="false"
                    placeholder="请输入"
                    @update:model-value="updateField('secret_key', $event)"
                />
                <template #extra>s3 secret_key</template>
            </a-form-item>
            <a-form-item label="bucket">
                <a-input
                    :model-value="modelValue.bucket"
                    :disabled="disabled"
                    :spellcheck="false"
                    placeholder="请输入"
                    @update:model-value="updateField('bucket', $event)"
                />
                <template #extra>s3 bucket</template>
            </a-form-item>
            <a-form-item label="host">
                <a-input
                    :model-value="modelValue.endpoint"
                    :disabled="disabled"
                    :spellcheck="false"
                    placeholder="请输入"
                    @update:model-value="updateField('endpoint', $event)"
                />
                <template #extra>s3 的 host 地址必须为集群应用内网地址</template>
            </a-form-item>
            <a-form-item label="地区">
                <a-input
                    :model-value="modelValue.region"
                    :disabled="disabled"
                    :spellcheck="false"
                    placeholder="请输入"
                    @update:model-value="updateField('region', $event)"
                />
                <template #extra>地区</template>
            </a-form-item>
        </a-form>
    </div>
</template>

<script>
export default {
    name: "StorageConfigForm",
    props: {
        modelValue: {
            type: Object,
            default: () => ({
                access_key: "",
                secret_key: "",
                bucket: "",
                endpoint: "",
                region: "",
            }),
        },
        disabled: {
            type: Boolean,
            default: false,
        },
    },
    emits: ["update:modelValue"],
    methods: {
        updateField(key, value) {
            this.$emit("update:modelValue", {
                ...this.modelValue,
                [key]: value,
            });
        },
    },
};
</script>

<style scoped>
.storage-config-form {
    max-width: 760px;
}
</style>
