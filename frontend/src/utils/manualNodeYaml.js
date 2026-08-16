import yaml from 'js-yaml'

export function serializeManualNode(node) {
  return yaml.dump(node || {}, {
    noCompatMode: true,
    lineWidth: -1,
    sortKeys: false,
  })
}

export function parseManualNodeYaml(content) {
  let node
  try {
    node = yaml.load(String(content || ''))
  } catch (error) {
    throw new Error(`节点 YAML 无法解析：${error.message}`)
  }
  if (!node || Array.isArray(node) || typeof node !== 'object') {
    throw new Error('节点配置必须是一个对象')
  }
  if (!String(node.name || '').trim()) {
    throw new Error('请填写节点名称')
  }
  return node
}
