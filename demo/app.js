const buttons = document.querySelectorAll('[data-panel]')
const panels = document.querySelectorAll('.panel')

buttons.forEach((button) => {
  button.addEventListener('click', () => {
    const target = button.dataset.panel
    buttons.forEach((item) => item.classList.toggle('active', item === button))
    panels.forEach((panel) => panel.classList.toggle('active', panel.id === `panel-${target}`))
  })
})

const repoLink = document.getElementById('repo-link')
if (repoLink && window.location.hostname.endsWith('github.io')) {
  const owner = window.location.hostname.replace('.github.io', '')
  const [repo] = window.location.pathname.split('/').filter(Boolean)
  if (owner && repo) repoLink.href = `https://github.com/${owner}/${repo}`
}
