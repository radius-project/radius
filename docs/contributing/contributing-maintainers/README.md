# Repository Access Management

This document provides guidance for Radius maintainers on managing repository access and permissions within the radius-project GitHub organization.

## Overview

The Radius project uses GitHub's built-in access control mechanisms to manage repository permissions. Access levels and roles are defined in the [community-membership.md](https://github.com/radius-project/community/blob/main/community-membership.md) document in the community repository.

## Roles and Permissions

The Radius project defines three main roles:

| Role | Permissions | Defined by |
|------|-------------|------------|
| **Member** | Active contributor, can be assigned issues and PRs | Radius GitHub org member |
| **Approver** | Can approve and merge PRs | CODEOWNERS file |
| **Maintainer** | Set direction and priorities, full repository access | CODEOWNERS file + GitHub repository settings |

## Giving Admin Access to a Repository

### Prerequisites

- You must be an organization owner or have admin access to the target repository
- The person requesting access should meet the requirements outlined in [community-membership.md](https://github.com/radius-project/community/blob/main/community-membership.md)

### Steps to Grant Admin Access

#### Option 1: Using GitHub Web Interface

1. **Navigate to the Repository**
   - Go to the target repository (e.g., `https://github.com/radius-project/resource-types-contrib`)

2. **Access Repository Settings**
   - Click on `Settings` in the repository navigation bar
   - Note: You must have admin permissions to see this option

3. **Manage Collaborators and Teams**
   - In the left sidebar, click `Collaborators and teams`
   - This shows all current collaborators and teams with access

4. **Add or Modify Access**
   - **For Individual Users:**
     - Click `Add people`
     - Search for the GitHub username
     - Select the role: `Admin`, `Maintain`, `Write`, or `Read`
     - Click `Add [username] to this repository`
   
   - **For Teams:**
     - Click `Add teams`
     - Search for the team name (e.g., `maintainers`)
     - Select the role level
     - Click `Add [team] to this repository`

5. **Update CODEOWNERS (if applicable)**
   - If the person is becoming a maintainer, update the `CODEOWNERS` file in the repository
   - Add their GitHub username to the appropriate sections
   - Create a pull request with the changes

#### Option 2: Using GitHub CLI

For maintainers who prefer command-line tools:

```bash
# Grant admin access to a user
gh api repos/radius-project/resource-types-contrib/collaborators/USERNAME \
  -X PUT \
  -f permission=admin

# Grant admin access to a team
gh api repos/radius-project/resource-types-contrib/teams/TEAM-SLUG \
  -X PUT \
  -f permission=admin
```

Replace:
- `resource-types-contrib` with the target repository name
- `USERNAME` with the GitHub username
- `TEAM-SLUG` with the team slug (lowercase, hyphenated team name)

### Verification

After granting access:

1. **Verify the Change**
   - Check the repository's collaborators page to confirm the user appears with the correct role
   - Ask the user to verify they can access the repository settings

2. **Document the Change**
   - If this is a maintainer addition, ensure it's documented in:
     - Repository CODEOWNERS file
     - Any relevant team documentation
     - Community membership records

3. **Notify the Team**
   - Announce the change in the team's communication channel (Discord, email, etc.)
   - Welcome the new maintainer/admin

## Specific Example: resource-types-contrib Repository

To give a member admin access to the `resource-types-contrib` repository:

1. **Verify Prerequisites**
   - Confirm the member meets the maintainer requirements
   - Check with existing maintainers for approval

2. **Grant Access**
   ```bash
   # Using GitHub CLI
   gh api repos/radius-project/resource-types-contrib/collaborators/USERNAME \
     -X PUT \
     -f permission=admin
   ```

   Or use the GitHub web interface as described above.

3. **Update CODEOWNERS**
   - Edit `.github/CODEOWNERS` in the resource-types-contrib repository
   - Add the username to the appropriate sections:
     ```
     # Example CODEOWNERS entry
     * @radius-project/maintainers @new-maintainer-username
     ```

4. **Create PR and Get Approval**
   - Submit the CODEOWNERS change as a pull request
   - Get approval from existing maintainers
   - Merge after approval

## Removing Access

To remove admin access:

1. Navigate to the repository settings → Collaborators and teams
2. Find the user or team
3. Click the role dropdown and select a lower permission level or click "Remove"
4. Update CODEOWNERS file if necessary

## Best Practices

1. **Principle of Least Privilege**: Grant the minimum access level required for the person to do their work
2. **Regular Audits**: Periodically review repository access and remove inactive collaborators
3. **Document Changes**: Keep the CODEOWNERS file up to date and document major access changes
4. **Team-Based Access**: Prefer granting access via teams rather than individual users when possible
5. **Follow Community Guidelines**: Always follow the process outlined in [community-membership.md](https://github.com/radius-project/community/blob/main/community-membership.md)

## Troubleshooting

### "I don't see the Settings tab"

You need admin access to the repository. Contact an organization owner or repository admin.

### "The user can't see the private repository"

Ensure the user is a member of the radius-project GitHub organization first. Organization membership is required before repository access can be granted.

### "Changes to CODEOWNERS aren't taking effect"

CODEOWNERS changes only take effect for new pull requests after the changes are merged to the default branch.

## Resources

- [GitHub Documentation: Repository Roles](https://docs.github.com/en/organizations/managing-user-access-to-your-organizations-repositories/repository-roles-for-an-organization)
- [Radius Community Membership Guidelines](https://github.com/radius-project/community/blob/main/community-membership.md)
- [GitHub CODEOWNERS Documentation](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners)
